package server

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

func (db *Database) Get(key string) (interface{}, bool) {
	response := make(chan readResponse, 1)
	db.enqueueReadTask(key, response)
	result := <-response
	return result.value, result.found
}

func (db *Database) enqueueReadTask(key string, response chan readResponse) {
	atomic.AddInt32(&db.readRequests, 1)
	db.readQueue <- readTask{key: key, response: response}
}

func (db *Database) processReadQueue() {
	var tasks []readTask
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case task := <-db.readQueue:
			tasks = append(tasks, task)
			db.adjustTicker(&ticker)

			if len(tasks) >= 100 {
				db.processBatch(tasks)
				tasks = nil
			}
		case <-ticker.C:
			if len(tasks) > 0 {
				db.processBatch(tasks)
				tasks = nil
			}
			db.adjustTicker(&ticker)
		}
	}
}

func (db *Database) adjustTicker(ticker **time.Ticker) {
	readRequests := atomic.LoadInt32(&db.readRequests)
	var interval time.Duration

	switch {
	case readRequests < 10:
		interval = 500 * time.Millisecond
	case readRequests < 100:
		interval = 250 * time.Millisecond
	case readRequests < 500:
		interval = 100 * time.Millisecond
	default:
		interval = 10 * time.Millisecond
	}

	*ticker = time.NewTicker(interval)
	atomic.StoreInt32(&db.readRequests, 0)
}

func (db *Database) processBatch(tasks []readTask) {
	db.tasksMu.Lock()
	defer db.tasksMu.Unlock()

	start := time.Now()
	keys := make([]string, len(tasks))
	for i, task := range tasks {
		keys[i] = task.key
	}

	results := db.batchRead(keys)

	for _, task := range tasks {
		value, found := results[task.key]
		task.response <- readResponse{value: value, found: found}
		close(task.response)
	}

	duration := time.Since(start).Milliseconds()
	db.avgReadTime = ((db.avgReadTime * float64(db.totalReadOperations)) + float64(duration)) / float64(db.totalReadOperations+1)
	atomic.AddInt32(&db.totalReadOperations, int32(len(tasks)))
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

func (db *Database) readFromFile(key string) (interface{}, bool) {
	db.mu.RLock()
	fileName, exists := db.keyToFileMap[key]
	db.mu.RUnlock()

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
