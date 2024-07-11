package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"pawiu-db/types"
	"strings"

	"gopkg.in/mgo.v2/bson"
)

type Database struct {
	Data map[string]interface{}
	file string
}

func NewDatabase(filename string) (*Database, error) {
	db := &Database{
		Data: make(map[string]interface{}),
		file: filepath.Join("db", filename),
	}
	err := db.load()
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (db *Database) load() error {
	// Loading BSON instead of JSON
	if _, err := os.Stat(db.file); os.IsNotExist(err) {
		return db.save()
	}

	data, err := os.ReadFile(db.file)
	if err != nil {
		return err
	}

	err = bson.Unmarshal(data, &db.Data)
	if err != nil {
		return err
	}

	return nil
}

func (db *Database) save() error {
	// Saving BSON instead of JSON
	if _, err := os.Stat("db"); os.IsNotExist(err) {
		err := os.Mkdir("db", 0755)
		if err != nil {
			return err
		}
	}

	data, err := bson.Marshal(db.Data)
	if err != nil {
		return err
	}

	err = os.WriteFile(db.file, data, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (db *Database) Set(key string, value interface{}) error {
	if key == "" {
		// Save/update in the main object
		db.Data = value.(map[string]interface{})
	} else {
		keys := strings.Split(key, ".")
		if len(keys) == 1 {
			db.Data[keys[0]] = value
		} else {
			nestedData := db.Data
			for i := 0; i < len(keys)-1; i++ {
				existingValue, ok := nestedData[keys[i]]
				if !ok {
					nestedData[keys[i]] = make(map[string]interface{})
					existingValue = nestedData[keys[i]]
				}
				nestedMap, ok := existingValue.(map[string]interface{})
				if !ok {
					return fmt.Errorf("key %s is not a map", strings.Join(keys[:i+1], "."))
				}
				nestedData = nestedMap
			}
			nestedData[keys[len(keys)-1]] = value
		}
	}
	return db.save()
}

func (db *Database) Get(key string) (interface{}, bool) {
	if key == "" {
		// Return the main object
		return db.Data, true
	}

	keys := strings.Split(key, ".")
	nestedData := db.Data
	for _, k := range keys {
		value, ok := nestedData[k]
		if !ok {
			return nil, false
		}
		if nested, nestedOk := value.(map[string]interface{}); nestedOk {
			nestedData = nested
		} else {
			return value, true
		}
	}
	return nestedData, true
}

func (db *Database) Delete(key string) error {
	keys := strings.Split(key, ".")
	if len(keys) == 1 {
		delete(db.Data, keys[0])
	} else {
		nestedData := db.Data
		for i := 0; i < len(keys)-1; i++ {
			if value, ok := nestedData[keys[i]]; ok {
				if nested, nestedOk := value.(map[string]interface{}); nestedOk {
					nestedData = nested
				} else {
					return fmt.Errorf("cannot delete key %s: intermediate key %s is not a map", key, keys[i])
				}
			} else {
				return fmt.Errorf("cannot delete key %s: intermediate key %s does not exist", key, keys[i])
			}
		}
		delete(nestedData, keys[len(keys)-1])
	}
	return db.save()
}

func (db *Database) Add(key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	keys := strings.Split(key, ".")
	if len(keys) == 1 {
		// Check if the key already exists
		if existingValue, ok := db.Data[keys[0]]; ok {
			// Check if the existing value is a map
			existingMap, ok := existingValue.(map[string]interface{})
			if !ok {
				return fmt.Errorf("key %s already exists and is not a map", keys[0])
			}
			// Merge new data into existing map
			for k, v := range value.(map[string]interface{}) {
				existingMap[k] = v
			}
		} else {
			// If the key doesn't exist, create a new entry
			db.Data[keys[0]] = value
		}
	} else {
		nestedData := db.Data
		for i := 0; i < len(keys)-1; i++ {
			existingValue, ok := nestedData[keys[i]]
			if !ok {
				nestedData[keys[i]] = make(map[string]interface{})
				existingValue = nestedData[keys[i]]
			}
			nestedMap, ok := existingValue.(map[string]interface{})
			if !ok {
				return fmt.Errorf("key %s is not a map", strings.Join(keys[:i+1], "."))
			}
			nestedData = nestedMap
		}
		// Check if the key already exists in the nested structure
		lastKey := keys[len(keys)-1]
		if existingValue, ok := nestedData[lastKey]; ok {
			// Check if the existing value is a map
			existingMap, ok := existingValue.(map[string]interface{})
			if !ok {
				return fmt.Errorf("key %s already exists and is not a map", lastKey)
			}
			// Merge new data into existing map
			for k, v := range value.(map[string]interface{}) {
				existingMap[k] = v
			}
		} else {
			// If the key doesn't exist, create a new entry
			nestedData[lastKey] = value
		}
	}

	return db.save()
}

func Start_server(config types.Config) {
	apiKey := config.Api_key
	db, err := NewDatabase("data.bson")
	if err != nil {
		panic(err)
	}

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
		if path == "" {
			data := db.Data
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
