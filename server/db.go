package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"pawiu-db/types"
	"runtime"
	"sync"

	"github.com/jroimartin/gocui"
)

var messageBuffer []string

type Database struct {
	data                map[string]*cacheData
	file                string
	useBSON             bool
	saveQueue           chan saveTask
	deleteQueue         chan deleteTask
	readQueue           chan readTask
	addQueue            chan addTask
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
	maxGoroutines       int
	batchSize           int
	readRequests        int32

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
		saveQueue:           make(chan saveTask, config.Queue_save_size),
		deleteQueue:         make(chan deleteTask, config.Queue_delete_size),
		readQueue:           make(chan readTask, config.Queue_read_size),
		addQueue:            make(chan addTask, config.Queue_add_size),
		addBufferSize:       1000,
		addBatchSize:        100,
		addBuffer:           make([]addTask, 0, 1000),
		maxKeysPerFile:      config.Max_keys_per_file,
		keyToFileMap:        make(map[string]string),
		currentFileKeyCount: make(map[string]int),
		maxGoroutines:       config.Max_goroutines,
		batchSize:           config.Batch_Size,
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

var gui *gocui.Gui

func StartServer(cfg types.Config, _gui *gocui.Gui) {
	config = cfg
	apiKey := config.Api_key
	gui = _gui
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

	displayMessage(fmt.Sprintf("File format: %s", name))
	displayMessage(fmt.Sprintf("Starting server on port %d", config.Port))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Server running on port %d", config.Port)
		// TODO: ping res
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
			// TODO err count
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

	http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		if !authAPIKey(r, apiKey) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		w.Write([]byte(config.Version))
	})

	go StartTCPServer(config, db)

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

// TCP =====================================
func StartTCPServer(cfg types.Config, db *Database) {
	address := fmt.Sprintf(":%d", cfg.Tcp_Port)
	displayMessage(fmt.Sprintf("TCP server started on %s", address))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		displayMessage(fmt.Sprintf("Failed to start TCP server: %v", err))
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			displayMessage(fmt.Sprintf("Failed to accept connection: %v", err))
			continue
		}
		go handleTCPConnection(conn, db)
	}
}

func handleTCPConnection(conn net.Conn, db *Database) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	decoder := json.NewDecoder(reader)

	for {
		var request map[string]interface{}
		err := decoder.Decode(&request)
		if err == io.EOF {
			log.Println("Connection closed by client")
			break
		}
		if err != nil {
			if nErr, ok := err.(net.Error); ok && nErr.Timeout() {
				log.Println("Connection timed out")
				break
			}
			if err == io.ErrUnexpectedEOF {
				log.Println("Connection closed abruptly")
				break
			}
			displayMessage(fmt.Sprintf("Invalid JSON format: %v", err))
			break
		}

		response := processRequest(request, db)
		responseBytes, err := json.Marshal(response)
		if err != nil {
			displayMessage(fmt.Sprintf("Failed to marshal response: %v", err))
			continue
		}

		_, err = conn.Write(append(responseBytes, '\n'))
		if err != nil {
			displayMessage(fmt.Sprintf("Failed to write response: %v", err))
			break
		}
	}
}

func processRequest(request map[string]interface{}, db *Database) map[string]interface{} {
	action, ok := request["action"].(string)
	if !ok {
		return map[string]interface{}{
			"error": "missing or invalid action",
		}
	}

	switch action {
	case "save":
		return handleSaveRequest(request, db)
	case "read":
		return handleReadRequest(request, db)
	case "delete":
		return handleDeleteRequest(request, db)
	case "add":
		return handleAddRequest(request, db)
	default:
		return map[string]interface{}{
			"error": "unknown action",
		}
	}
}

func handleSaveRequest(request map[string]interface{}, db *Database) map[string]interface{} {
	path, ok := request["path"].(string)
	if !ok {
		return map[string]interface{}{
			"error": "missing or invalid path",
		}
	}
	data, ok := request["data"].(interface{})
	if !ok {
		return map[string]interface{}{
			"error": "missing or invalid data",
		}
	}
	err := db.Set(path, data)
	if err != nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("failed to save data: %v", err),
		}
	}
	return map[string]interface{}{
		"status": "ok",
	}
}

func handleReadRequest(request map[string]interface{}, db *Database) map[string]interface{} {
	path, ok := request["path"].(string)
	if !ok {
		return map[string]interface{}{
			"error": "missing or invalid path",
		}
	}
	data, found := db.Get(path)
	if !found {
		return map[string]interface{}{
			"error": "data not found",
		}
	}
	return map[string]interface{}{
		"status": "ok",
		"data":   data,
	}
}

func handleDeleteRequest(request map[string]interface{}, db *Database) map[string]interface{} {
	path, ok := request["path"].(string)
	if !ok {
		return map[string]interface{}{
			"error": "missing or invalid path",
		}
	}
	err := db.Delete(path)
	if err != nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("failed to delete data: %v", err),
		}
	}
	return map[string]interface{}{
		"status": "ok",
	}
}

func handleAddRequest(request map[string]interface{}, db *Database) map[string]interface{} {
	path, ok := request["path"].(string)
	if !ok {
		return map[string]interface{}{
			"error": "missing or invalid path",
		}
	}
	data, ok := request["data"].(interface{})
	if !ok {
		return map[string]interface{}{
			"error": "missing or invalid data",
		}
	}
	err := db.Add(path, data)
	if err != nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("failed to add data: %v", err),
		}
	}
	return map[string]interface{}{
		"status": "ok",
	}
}
