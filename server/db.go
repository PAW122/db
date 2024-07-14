package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"pawiu-db/types"
	"sync"
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
	data      map[string]*cacheData
	file      string
	useBSON   bool
	saveQueue chan saveTask
	mu        sync.Mutex
}

type cacheData struct {
	Value interface{}
}

type saveTask struct {
	key   string
	value interface{}
}

func NewDatabase(filename string, useBSON bool) (*Database, error) {
	db := &Database{
		data:      make(map[string]*cacheData),
		file:      filepath.Join("db", filename),
		useBSON:   useBSON,
		saveQueue: make(chan saveTask, 10000),
	}
	err := db.load()
	if err != nil {
		return nil, err
	}
	go db.processSaveQueue()
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
	db.mu.Lock()
	defer db.mu.Unlock()

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
			tasks[task.key] = task.value
			if len(tasks) >= 100 {
				if err := db.batchSave(tasks); err != nil {
					log.Printf("Error saving batch: %v", err)
				}
				tasks = make(map[string]interface{})
			}
		case <-ticker.C:
			if len(tasks) > 0 {
				if err := db.batchSave(tasks); err != nil {
					log.Printf("Error saving batch: %v", err)
				}
				tasks = make(map[string]interface{})
			}
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

func (db *Database) enqueueSaveTask(key string, value interface{}) {
	db.saveQueue <- saveTask{key: key, value: value}
	// log.Printf("Enqueued save task: %s", key)
}

func (db *Database) Set(key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	db.enqueueSaveTask(key, value)
	return nil
}

func (db *Database) Get(key string) (interface{}, bool) {
	cachedData, cacheFound := getCache(key)
	if cacheFound {
		return cachedData, true
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	cd, ok := db.data[key]
	if !ok {
		return nil, false
	}

	go cacheOutgoing(key, cd.Value)
	return cd.Value, true
}

func (db *Database) Delete(key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	delete(db.data, key)
	return db.save()
}

func (db *Database) Add(key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	db.enqueueSaveTask(key, value)
	return nil
}

func StartServer(cfg types.Config) {
	config = cfg
	apiKey := config.Api_key
	var name string
	if config.UseBSON {
		name = "data.bson"
	} else {
		name = "data.json"
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
			http.Error(w, "Invalid JSON format", http.StatusBadRequest)
			return
		}

		path := r.URL.Query().Get("path")
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
