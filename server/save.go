package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/mgo.v2/bson"
)

func (db *Database) Set(key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	db.enqueueSaveTask(key, value)
	return nil
}

func (db *Database) enqueueSaveTask(key string, value interface{}) {
	db.saveQueue <- saveTask{key: key, value: value}
}

func (db *Database) batchSave(tasks map[string]interface{}) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	filesToTasks := make(map[string]map[string]interface{})
	for key, value := range tasks {
		file, exists := db.keyToFileMap[key]
		if !exists {
			if db.currentFile == "" || db.currentFileKeyCount[db.currentFile] >= db.maxKeysPerFile {
				db.currentFile = db.generateNewFileName()
				db.currentFileKeyCount[db.currentFile] = 0
			}
			file = db.currentFile
			db.keyToFileMap[key] = file
		}

		if _, ok := filesToTasks[file]; !ok {
			filesToTasks[file] = make(map[string]interface{})
		}
		filesToTasks[file][key] = value
		db.currentFileKeyCount[file]++
	}

	var wg sync.WaitGroup
	fileChannel := make(chan struct{}, 10) // Zwiększona liczba równoczesnych operacji plikowych

	saveFile := func(file string, tasks map[string]interface{}) {
		defer wg.Done()
		fileChannel <- struct{}{}
		if err := db.appendToFile(file, tasks); err != nil {
			log.Printf("Error appending to file %v: %v", file, err)
		}
		<-fileChannel
	}

	for file, tasks := range filesToTasks {
		wg.Add(1)
		go saveFile(file, tasks)
	}
	wg.Wait()

	return nil
}

func (db *Database) appendToFile(file string, newTasks map[string]interface{}) error {
	filePath := filepath.Join("db", file)

	var existingData map[string]interface{}
	if _, err := os.Stat(filePath); err == nil {
		existingData, err = readFile(filePath)
		if err != nil {
			return fmt.Errorf("error reading file: %v", err)
		}
	} else {
		existingData = make(map[string]interface{})
	}

	// Update existing data with new tasks
	for key, value := range newTasks {
		updateNestedMap(existingData, key, value)
	}

	var data []byte
	var err error
	if db.useBSON {
		data, err = bson.Marshal(existingData)
	} else {
		data, err = json.Marshal(existingData)
	}
	if err != nil {
		return fmt.Errorf("error marshalling data: %v", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("error writing to file: %v", err)
	}

	return nil
}

func (db *Database) generateNewFileName() string {
	newFileIndex := len(db.currentFileKeyCount) + 1
	if db.useBSON {
		return "db_file_" + strconv.Itoa(newFileIndex) + ".bson"
	} else {
		return "db_file_" + strconv.Itoa(newFileIndex) + ".json"
	}
}

func (db *Database) processSaveQueue() {
	tasks := make(map[string]interface{})
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case task := <-db.saveQueue:
			db.tasksMu.Lock()
			start := time.Now()
			tasks[task.key] = task.value
			if len(tasks) >= 100 { // TODO z connfigu pobierać wartość albo ją dynamicznie dostosowywać
				if err := db.batchSave(tasks); err != nil {
					log.Printf("Error saving batch: %v", err)
				}
				go cacheIncoming(task.key, task.value)
				tasks = make(map[string]interface{})
			}
			db.tasksMu.Unlock()

			//stats
			duration := time.Since(start).Milliseconds()
			db.avgSaveTime = ((db.avgSaveTime * float64(db.totalSaveOperations)) + float64(duration)) / float64(db.totalSaveOperations+1)
			atomic.AddInt32(&db.totalSaveOperations, 1)
		case <-ticker.C:
			db.tasksMu.Lock()
			start := time.Now()
			if len(tasks) > 0 {
				if err := db.batchSave(tasks); err != nil {
					log.Printf("Error saving batch: %v", err)
				}
				tasks = make(map[string]interface{})

				//stats
				duration := time.Since(start).Milliseconds()
				db.avgSaveTime = ((db.avgSaveTime * float64(db.totalSaveOperations)) + float64(duration)) / float64(db.totalSaveOperations+1)
				atomic.AddInt32(&db.totalSaveOperations, 1)
			}
			db.tasksMu.Unlock()
		}
	}
}
func readFile(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func (db *Database) save() error {
	db.saveMu.Lock()
	defer db.saveMu.Unlock()

	if _, err := os.Stat("db"); os.IsNotExist(err) {
		if err := os.Mkdir("db", 0755); err != nil {
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

	if err := os.WriteFile(db.file, data, 0644); err != nil {
		return err
	}

	return nil
}

func updateNestedMap(data map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")
	lastIndex := len(parts) - 1

	for i, part := range parts {
		if i == lastIndex {
			data[part] = value
		} else {
			if _, exists := data[part]; !exists {
				data[part] = make(map[string]interface{})
			}
			data = data[part].(map[string]interface{})
		}
	}
}
