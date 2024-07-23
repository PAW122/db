package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"pawiu-db/types"
	"runtime"
	"sync"
)

// TODO: dodac spowrotem obsługę bson
type Database struct {
	data                map[string]*cacheData
	file                string
	useBSON             bool
	saveQueue           chan saveTask
	deleteQueue         chan deleteTask
	readQueue           chan readTask
	addQueue            chan addTask
	batchReadQueue      chan batchReadTask
	mu                  sync.RWMutex
	saveMu              sync.Mutex
	deleteMu            sync.Mutex
	tasksMu             sync.Mutex
	wg                  sync.WaitGroup
	addBufferSize       int
	addBatchSize        int
	addBuffer           []addTask
	addBufferMutex      sync.Mutex
	addBufferCond       *sync.Cond
	saveWorkers         int32
	deleteWorkers       int32
	readWorkers         int32
	addWorkers          int32
	keyToFileMap        map[string]string
	currentFile         string
	maxKeysPerFile      int
	currentFileKeyCount map[string]int
	cache               map[string]interface{}
	cacheMu             sync.RWMutex

	//gui stats
	totalSaveOperations   int32
	totalReadOperations   int32
	totalDeleteOperations int32
	totalAddOperations    int32
	avgSaveTime           float64
	avgReadTime           float64
	avgDeleteTime         float64
	avgAddTime            float64
}

type batchReadTask struct {
	keys     []string
	response chan map[string]interface{}
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
		data:                make(map[string]*cacheData),
		file:                filepath.Join("db", filename),
		useBSON:             useBSON,
		saveQueue:           make(chan saveTask, 100),
		deleteQueue:         make(chan deleteTask, 100),
		readQueue:           make(chan readTask, 100),
		addQueue:            make(chan addTask, 100),
		addBufferSize:       1000,
		addBatchSize:        100,
		addBuffer:           make([]addTask, 0, 1000),
		maxKeysPerFile:      10000,
		keyToFileMap:        make(map[string]string),
		currentFileKeyCount: make(map[string]int),
	}
	db.addBufferCond = sync.NewCond(&db.addBufferMutex)
	err := db.load()
	if err != nil {
		return nil, err
	}

	numCPU := runtime.NumCPU()

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

	var file_ext string
	if db.useBSON {
		file_ext = ".bson"
	} else {
		file_ext = ".json"
	}

	files, err := filepath.Glob(filepath.Join("db", "db_file_*"+file_ext))
	if err != nil {
		return err
	}

	for _, file := range files {
		existingData, err := readFile(file)
		if err != nil {
			log.Printf("Error reading file %v: %v", file, err)
			continue
		}
		for key := range existingData {
			db.keyToFileMap[key] = filepath.Base(file)
		}
		db.currentFileKeyCount[filepath.Base(file)] = len(existingData)
	}

	return nil
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
