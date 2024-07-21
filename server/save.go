package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
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

	// Prepare tasks for concurrent saving
	var wg sync.WaitGroup
	fileChannel := make(chan string, 2)

	// Function to save tasks for a specific file
	saveFile := func(file string, tasks map[string]interface{}) {
		defer wg.Done()
		if err := db.appendToFile(file, tasks); err != nil {
			log.Printf("Error appending to file %v: %v", file, err)
		}
	}

	// Start saving tasks for up to 2 files concurrently
	for file, tasks := range filesToTasks {
		wg.Add(1)
		go saveFile(file, tasks)
		fileChannel <- file
		if len(fileChannel) >= 2 {
			<-fileChannel // Limit the number of concurrent file operations to 2
		}
	}
	wg.Wait()

	return nil
}

func (db *Database) appendToFile(file string, newTasks map[string]interface{}) error {
	filePath := filepath.Join("db", file)

	// Read existing data from file
	var existingData map[string]interface{}
	if _, err := os.Stat(filePath); err == nil {
		existingData, err = readFile(filePath)
		if err != nil {
			return fmt.Errorf("error reading file: %v", err)
		}
	} else {
		existingData = make(map[string]interface{})
	}

	// Merge new tasks with existing data
	for key, value := range newTasks {
		existingData[key] = value
	}

	// Marshal updated data
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

	// Write updated data back to file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("error writing to file: %v", err)
	}

	return nil
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

func (db *Database) generateNewFileName() string {
	newFileIndex := len(db.currentFileKeyCount) + 1
	if db.useBSON {
		return "db_file_" + strconv.Itoa(newFileIndex) + ".bson"
	} else {
		return "db_file_" + strconv.Itoa(newFileIndex) + ".json"
	}
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

func (db *Database) processSaveQueue() {
	tasks := make(map[string]interface{})
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case task := <-db.saveQueue:
			db.tasksMu.Lock()
			tasks[task.key] = task.value
			if len(tasks) >= 100 {
				if err := db.batchSave(tasks); err != nil {
					log.Printf("Error saving batch: %v", err)
				}
				tasks = make(map[string]interface{})
			}
			db.tasksMu.Unlock()
		case <-ticker.C:
			db.tasksMu.Lock()
			if len(tasks) > 0 {
				if err := db.batchSave(tasks); err != nil {
					log.Printf("Error saving batch: %v", err)
				}
				tasks = make(map[string]interface{})
			}
			db.tasksMu.Unlock()
		}
	}
}
