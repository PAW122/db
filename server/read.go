package server

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

func (db *Database) Get(key string) (interface{}, bool) {
	response := make(chan readResponse)
	db.enqueueReadTask(key, response)
	result := <-response
	return result.value, result.found
}

func (db *Database) enqueueReadTask(key string, response chan readResponse) {
	db.readQueue <- readTask{key: key, response: response}
}

func (db *Database) processReadQueue() {
	for task := range db.readQueue {
		db.processSingleReadTask(task)
	}
}

func (db *Database) processSingleReadTask(task readTask) {
	db.cacheMu.RLock()
	cachedData, cacheFound := db.cache[task.key]
	db.cacheMu.RUnlock()
	if cacheFound {
		task.response <- readResponse{value: cachedData, found: true}
		return
	}

	db.mu.Lock()
	cd, ok := db.data[task.key]
	db.mu.Unlock()

	if !ok {
		data, found := db.readFromFile(task.key)
		if !found {
			task.response <- readResponse{value: nil, found: false}
			return
		}
		go cacheOutgoing(task.key, data)
		task.response <- readResponse{value: data, found: true}
	} else {
		go cacheOutgoing(task.key, cd)
		task.response <- readResponse{value: cd, found: true}
	}
}

func (db *Database) readFromFile(key string) (interface{}, bool) {
	db.mu.Lock()
	fileName, exists := db.keyToFileMap[key]
	db.mu.Unlock()

	if !exists {
		return nil, false
	}

	filePath := filepath.Join("db", fileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false
	}

	var fileData map[string]interface{}
	if err := json.Unmarshal(data, &fileData); err != nil {
		return nil, false
	}

	value, found := fileData[key]
	return value, found
}

func (db *Database) batchRead(keys []string) map[string]interface{} {
	results := make(map[string]interface{})
	fileKeyMap := make(map[string][]string)

	for _, key := range keys {
		fileName, exists := db.keyToFileMap[key]
		if exists {
			fileKeyMap[fileName] = append(fileKeyMap[fileName], key)
		}
	}

	var wg sync.WaitGroup
	mu := &sync.Mutex{}

	readFromFile := func(fileName string, keys []string) {
		defer wg.Done()
		filePath := filepath.Join("db", fileName)
		data, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("Error reading file %v: %v", filePath, err)
			return
		}

		var fileData map[string]interface{}
		if err := json.Unmarshal(data, &fileData); err != nil {
			log.Printf("Error unmarshalling data from file %v: %v", filePath, err)
			return
		}

		mu.Lock()
		for _, key := range keys {
			if value, found := fileData[key]; found {
				results[key] = value
			}
		}
		mu.Unlock()
	}

	for fileName, keys := range fileKeyMap {
		wg.Add(1)
		go readFromFile(fileName, keys)
	}

	wg.Wait()
	return results
}
