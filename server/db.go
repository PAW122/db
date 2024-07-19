package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"pawiu-db/types"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/patrickmn/go-cache"
	"gopkg.in/mgo.v2/bson"
)

// Cache
var c *cache.Cache
var config types.Config

func init() {
	c = cache.New(5*time.Minute, 10*time.Minute)
}

func getCache(key string) (interface{}, bool) {
	if !config.Cache_incoming_all && !config.Cache_outgoing_all {
		return nil, false
	}
	cachedData, found := c.Get(key)
	return cachedData, found
}

func cacheIncoming(key string, data interface{}) bool {
	if config.Cache_incoming_all {
		duration := time.Duration(config.Cache_incoming_time) * time.Second
		c.Set(key, data, duration)
		return true
	}
	return false
}

func cacheOutgoing(key string, data interface{}) bool {
	if config.Cache_outgoing_all {
		duration := time.Duration(config.Cache_outgoing_time) * time.Second
		c.Set(key, data, duration)
		return true
	}
	return false
}

type Database struct {
	data           map[string]*cacheData
	file           string
	useBSON        bool
	saveQueue      chan saveTask
	deleteQueue    chan deleteTask
	readQueue      chan readTask
	addQueue       chan addTask
	mu             sync.Mutex
	saveMu         sync.Mutex
	deleteMu       sync.Mutex
	tasksMu        sync.Mutex
	wg             sync.WaitGroup
	addBufferSize  int
	addBatchSize   int
	addBuffer      []addTask
	addBufferMutex sync.Mutex
	addBufferCond  *sync.Cond
	saveWorkers    int32
	deleteWorkers  int32
	readWorkers    int32
	addWorkers     int32
}

type addTask struct {
	key   string
	value interface{}
}

type cacheData struct {
	Value interface{}
}

type saveTask struct {
	key   string
	value interface{}
}

type deleteTask struct {
	key string
}

type readTask struct {
	key      string
	response chan readResponse
}

type readResponse struct {
	value interface{}
	found bool
}

func NewDatabase(filename string, useBSON bool) (*Database, error) {
	db := &Database{
		data:          make(map[string]*cacheData),
		file:          filepath.Join(config.File_name, filename),
		useBSON:       useBSON,
		saveQueue:     make(chan saveTask, config.Queue_save_size),
		deleteQueue:   make(chan deleteTask, config.Queue_delete_size),
		readQueue:     make(chan readTask, config.Queue_read_size),
		addQueue:      make(chan addTask, config.Queue_add_size),
		addBufferSize: 1000,
		addBatchSize:  100,
		addBuffer:     make([]addTask, 0, 1000),
	}
	db.addBufferCond = sync.NewCond(&db.addBufferMutex)
	err := db.load()
	if err != nil {
		return nil, err
	}

	numCPU := runtime.NumCPU() * config.AsqsConfig.Worker_count_multiplier

	for i := 0; i < numCPU; i++ {
		db.wg.Add(1)
		go db.processSaveQueue()
		db.wg.Add(1)
		go db.processDeleteQueue()
		db.wg.Add(1)
		go db.processReadQueue()
		db.wg.Add(1)
		go db.processAddQueue()
	}

	go db.autoScaleWorkers()

	go AutoScalingFileSystem(config)
	return db, nil
}

func (db *Database) load() error {
	if _, err := os.Stat(db.file); os.IsNotExist(err) {
		return db.save()
	}

	data, err := os.ReadFile(db.file)
	if err != nil {
		return err
	}

	if db.useBSON {
		err = bson.Unmarshal(data, &db.data)
	} else {
		err = json.Unmarshal(data, &db.data)
	}
	if err != nil {
		return err
	}

	return nil
}

func (db *Database) save() error {
	db.saveMu.Lock()
	defer func() {
		db.saveMu.Unlock()
	}()

	if _, err := os.Stat("db"); os.IsNotExist(err) {
		err := os.Mkdir("db", 0755)
		if err != nil {
			return err
		}
	}

	var data []byte
	var err error
	if db.useBSON {
		data, err = bson.Marshal(db.data)
	} else {
		data, err = json.Marshal(db.data)
	}
	if err != nil {
		return err
	}

	err = os.WriteFile(db.file, data, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (db *Database) processSaveQueue() {
	tasks := make(map[string]interface{})
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case task := <-db.saveQueue:
			db.tasksMu.Lock()
			tasks[task.key] = task.value
			if len(tasks) >= 100 {
				if err := db.batchSave(tasks); err != nil {
					log.Printf("Error saving batch: %v", err)
				}
				tasks = make(map[string]interface{})
			}
			db.tasksMu.Unlock()
		case <-ticker.C:
			db.tasksMu.Lock()
			if len(tasks) > 0 {
				if err := db.batchSave(tasks); err != nil {
					log.Printf("Error saving batch: %v", err)
				}
				tasks = make(map[string]interface{})
			}
			db.tasksMu.Unlock()
		}
	}
}

func (db *Database) processReadQueue() {
	for task := range db.readQueue {
		cachedData, cacheFound := getCache(task.key)
		if cacheFound {
			task.response <- readResponse{value: cachedData, found: true}
			continue
		}

		db.mu.Lock()
		cd, ok := db.data[task.key]
		db.mu.Unlock()

		if !ok {
			task.response <- readResponse{value: nil, found: false}
		} else {
			go cacheOutgoing(task.key, cd.Value)
			task.response <- readResponse{value: cd.Value, found: true}
		}
	}
}

func (db *Database) processDeleteQueue() {
	tasks := make(map[string]struct{})
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case task := <-db.deleteQueue:
			db.tasksMu.Lock() // Zablokowanie dostępu do mapy tasks
			tasks[task.key] = struct{}{}
			if len(tasks) >= 100 {
				if err := db.batchDelete(tasks); err != nil {
					log.Printf("Error deleting batch: %v", err)
				}
				tasks = make(map[string]struct{})
			}
			db.tasksMu.Unlock() // Odblokowanie dostępu do mapy tasks
		case <-ticker.C:
			db.tasksMu.Lock() // Zablokowanie dostępu do mapy tasks
			if len(tasks) > 0 {
				if err := db.batchDelete(tasks); err != nil {
					log.Printf("Error deleting batch: %v", err)
				}
				tasks = make(map[string]struct{})
			}
			db.tasksMu.Unlock() // Odblokowanie dostępu do mapy tasks
		}
	}
}

func (db *Database) batchSave(tasks map[string]interface{}) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	for key, value := range tasks {
		cd, ok := db.data[key]
		if !ok {
			cd = &cacheData{}
			db.data[key] = cd
		}
		cd.Value = value
	}

	if _, err := os.Stat("db"); os.IsNotExist(err) {
		err := os.Mkdir("db", 0755)
		if err != nil {
			log.Printf("Error creating directory: %v", err)
			return err
		}
	}

	var data []byte
	var err error
	if db.useBSON {
		data, err = bson.Marshal(db.data)
	} else {
		data, err = json.Marshal(db.data)
	}
	if err != nil {
		log.Printf("Error marshalling data: %v", err)
		return err
	}

	err = os.WriteFile(db.file, data, 0644)
	if err != nil {
		log.Printf("Error writing to file: %v", err)
		return err
	}

	return nil
}

func (db *Database) batchDelete(tasks map[string]struct{}) error {
	db.deleteMu.Lock()
	defer db.deleteMu.Unlock()

	for key := range tasks {
		delete(db.data, key)
	}

	err := db.save()
	return err
}

func (db *Database) Delete(key string) error {
	db.deleteQueue <- deleteTask{key: key}
	return nil
}

func (db *Database) enqueueSaveTask(key string, value interface{}) {
	db.saveQueue <- saveTask{key: key, value: value}
}

func (db *Database) enqueueReadTask(key string, response chan readResponse) {
	db.readQueue <- readTask{key: key, response: response}
}

func (db *Database) Set(key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	db.enqueueSaveTask(key, value)
	return nil
}

func (db *Database) Get(key string) (interface{}, bool) {
	response := make(chan readResponse)
	db.enqueueReadTask(key, response)
	result := <-response
	return result.value, result.found
}

func (db *Database) Add(key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	db.addQueue <- addTask{key: key, value: value}
	return nil
}

func (db *Database) processAddBuffer() {
	db.addBufferMutex.Lock()
	defer db.addBufferMutex.Unlock()

	batchSize := db.addBatchSize
	if len(db.addBuffer) < batchSize {
		batchSize = len(db.addBuffer)
	}

	tasks := db.addBuffer[:batchSize]
	db.addBuffer = db.addBuffer[batchSize:]

	for _, task := range tasks {
		cache_data, cache_found := getCache(task.key)
		var existingValue map[string]interface{}
		if cache_found {
			existingValue, _ = cache_data.(map[string]interface{})
		} else {
			data, found := db.Get(task.key)
			if found {
				existingValue, _ = data.(map[string]interface{})
			}
		}

		if existingValue == nil {
			existingValue = make(map[string]interface{})
		}

		newValue, ok := task.value.(map[string]interface{})
		if !ok {
			continue
		}

		for k, v := range newValue {
			existingValue[k] = v
		}

		db.enqueueSaveTask(task.key, existingValue)
	}
}

func (db *Database) processAddQueue() {
	ticker := time.NewTicker(time.Duration(config.AsqsConfig.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case task := <-db.addQueue:
			db.addBufferMutex.Lock()
			db.addBuffer = append(db.addBuffer, task)
			if len(db.addBuffer) >= db.addBufferSize {
				db.addBufferCond.Signal()
			}
			db.addBufferMutex.Unlock()

		case <-ticker.C:
			db.addBufferMutex.Lock()
			if len(db.addBuffer) > 0 {
				db.processAddBuffer()
			}
			db.addBufferMutex.Unlock()
		}
	}
}

func GetQueueLength(db *Database) int {
	return len(db.saveQueue)
}

func GetQueueLengthDelete(db *Database) int {
	return len(db.deleteQueue)
}

func GetQueueLengthRead(db *Database) int {
	return len(db.readQueue)
}

func (db *Database) autoScaleWorkers() {
	ticker := time.NewTicker(time.Duration(config.AsqsConfig.Interval) * time.Second)
	defer ticker.Stop()

	firstRun := true

	for range ticker.C {
		saveQueueLen := len(db.saveQueue)
		deleteQueueLen := len(db.deleteQueue)
		readQueueLen := len(db.readQueue)
		addQueueLen := len(db.addQueue)

		if saveQueueLen > config.AsqsConfig.Queue_threshold {
			go db.processSaveQueue()
			atomic.AddInt32(&db.saveWorkers, 1)
		}
		if deleteQueueLen > config.AsqsConfig.Queue_threshold {
			go db.processDeleteQueue()
			atomic.AddInt32(&db.deleteWorkers, 1)
		}
		if readQueueLen > config.AsqsConfig.Queue_threshold {
			go db.processReadQueue()
			atomic.AddInt32(&db.readWorkers, 1)
		}
		if addQueueLen > config.AsqsConfig.Queue_threshold {
			go db.processAddQueue()
			atomic.AddInt32(&db.addWorkers, 1)
		}

		// Print statistics to the terminal
		if firstRun {
			clearTerminal()
			firstRun = false
		} else {
			moveCursorUp(4) // Zwiększ o 4 linie dla dodania nowej sekcji
		}
		fmt.Printf("Save Queue Length: %d, Workers: %d\n", saveQueueLen, atomic.LoadInt32(&db.saveWorkers))
		fmt.Printf("Add Queue Length: %d, Workers: %d\n", addQueueLen, atomic.LoadInt32(&db.addWorkers))
		fmt.Printf("Delete Queue Length: %d, Workers: %d\n", deleteQueueLen, atomic.LoadInt32(&db.deleteWorkers))
		fmt.Printf("Read Queue Length: %d, Workers: %d\n", readQueueLen, atomic.LoadInt32(&db.readWorkers))
	}
}

// Function to clear the terminal screen
func clearTerminal() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// Function to move the cursor up by n lines
func moveCursorUp(n int) {
	fmt.Printf("\033[%dA", n)
}

func StartServer(cfg types.Config) {
	config = cfg
	apiKey := config.Api_key
	var name string
	if config.UseBSON {
		name = config.File_name + ".bson"
	} else {
		name = config.File_name + ".json"
	}
	db, err := NewDatabase(name, config.UseBSON)
	if err != nil {
		panic(err)
	}

	fmt.Printf("File format: %s\n", name)
	fmt.Printf("Starting server on port %d\n", config.Port)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Server running on port %d", config.Port)
	})

	http.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if !authAPIKey(r, apiKey) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var data interface{}
		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, "Invalid JSON format", http.StatusBadRequest)
			return
		}

		path := r.URL.Query().Get("path")
		go cacheIncoming(path, data)
		err = db.Set(path, data)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to save data: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/read", func(w http.ResponseWriter, r *http.Request) {
		if !authAPIKey(r, apiKey) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		path := r.URL.Query().Get("path")

		cache_data, cache_found := getCache(path)
		if cache_found {
			sendJSONResponse(w, cache_data)
			return
		}

		if path == "" {
			db.mu.Lock()
			data := db.data
			db.mu.Unlock()
			sendJSONResponse(w, data)
			return
		}

		data, found := db.Get(path)
		if !found {
			http.Error(w, "Data not found", http.StatusNotFound)
			return
		}

		sendJSONResponse(w, data)
	})

	http.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request) {
		if !authAPIKey(r, apiKey) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "Path parameter is required", http.StatusBadRequest)
			return
		}

		err := db.Delete(path)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to delete data: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/add", func(w http.ResponseWriter, r *http.Request) {
		if !authAPIKey(r, apiKey) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var data interface{}
		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			log.Printf("Invalid JSON format: %v", err)
			http.Error(w, "Invalid JSON format", http.StatusBadRequest)
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "Path parameter is required", http.StatusBadRequest)
			return
		}

		go cacheIncoming(path, data)
		err = db.Add(path, data)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to add data: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil)
}

func authAPIKey(r *http.Request, apiKey string) bool {
	providedApiKey := r.Header.Get("X-API-Key")
	return providedApiKey == apiKey
}

func sendJSONResponse(w http.ResponseWriter, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to serialize data to JSON: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}
