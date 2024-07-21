package server

import (
	"fmt"
	"time"
)

func (db *Database) processAddQueue() {
	ticker := time.NewTicker(time.Duration(1) * time.Second)
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
