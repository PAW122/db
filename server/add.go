package server

import (
	"fmt"
	"sync/atomic"
	"time"
)

func (db *Database) processAddQueue() {
	ticker := time.NewTicker(time.Duration(1) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case task := <-db.addQueue:
			db.addBufferMutex.Lock()
			start := time.Now()
			db.addBuffer = append(db.addBuffer, task)
			if len(db.addBuffer) >= db.addBufferSize {
				db.addBufferCond.Signal()
			}
			db.addBufferMutex.Unlock()
			//stats
			duration := time.Since(start).Milliseconds()
			db.avgAddTime = ((db.avgAddTime * float64(db.totalAddOperations)) + float64(duration)) / float64(db.totalAddOperations+1)
			atomic.AddInt32(&db.totalAddOperations, 1)

		case <-ticker.C:
			db.addBufferMutex.Lock()
			start := time.Now()
			if len(db.addBuffer) > 0 {
				db.processAddBuffer()

				//stats
				duration := time.Since(start).Milliseconds()
				db.avgAddTime = ((db.avgAddTime * float64(db.totalAddOperations)) + float64(duration)) / float64(db.totalAddOperations+1)
				atomic.AddInt32(&db.totalAddOperations, 1)
			}
			db.addBufferMutex.Unlock()
		}
	}
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
