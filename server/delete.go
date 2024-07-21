package server

import (
	"log"
	"time"
)

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
