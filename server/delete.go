package server

import (
	"log"
	"sync/atomic"
	"time"
)

func (db *Database) processDeleteQueue() {
	tasks := make(map[string]struct{})
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case task := <-db.deleteQueue:
			db.tasksMu.Lock()
			start := time.Now()
			tasks[task.key] = struct{}{}
			if len(tasks) >= 100 {
				if err := db.batchDelete(tasks); err != nil {
					log.Printf("Error deleting batch: %v", err)
				}
				tasks = make(map[string]struct{})
			}
			db.tasksMu.Unlock()
			//stats
			duration := time.Since(start).Milliseconds()
			atomic.AddInt32(&db.totalDeleteOperations, 1)
			if db.totalDeleteOperations > 0 {
				db.avgDeleteTime = ((db.avgDeleteTime * float64(db.totalDeleteOperations-1)) + float64(duration)) / float64(db.totalDeleteOperations)
			}
		case <-ticker.C:
			db.tasksMu.Lock()
			start := time.Now()
			if len(tasks) > 0 {
				if err := db.batchDelete(tasks); err != nil {
					log.Printf("Error deleting batch: %v", err)
				}
				tasks = make(map[string]struct{})

				//stats
				duration := time.Since(start).Milliseconds()
				db.avgDeleteTime = ((db.avgDeleteTime * float64(db.totalDeleteOperations)) + float64(duration)) / float64(db.totalDeleteOperations+1)
				atomic.AddInt32(&db.totalDeleteOperations, 1)
			}
			db.tasksMu.Unlock()
		}
	}
}

func (db *Database) batchDelete(tasks map[string]struct{}) error {
	db.deleteMu.Lock()
	defer db.deleteMu.Unlock()

	for key := range tasks {
		delete(db.data, key)
	}

	err := db.save()
	if err != nil {
		log.Printf("Error saving data after delete: %v", err)
	}
	return err
}

func (db *Database) Delete(key string) error {
	db.deleteQueue <- deleteTask{key: key}
	return nil
}
