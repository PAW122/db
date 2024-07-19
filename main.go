package main

import (
	"encoding/json"
	"fmt"
	"os"
	server "pawiu-db/server"
	types "pawiu-db/types"
)

func main() {
	const configFileName = "database_config.json"

	// Sprawdź, czy plik istnieje
	if _, err := os.Stat(configFileName); os.IsNotExist(err) {
		// Plik nie istnieje, więc go tworzymy
		fmt.Println("File config.json dont exist. Creating new file...")

		// Przykładowa konfiguracja
		config := types.Config{
			Port:                5432,
			Api_key:             "database_acces_api_key",
			Version:             "1.2.4",
			File_name:           "db",
			Cache_incoming_all:  true,
			Cache_incoming_time: 120,
			Cache_outgoing_all:  true,
			Cache_outgoing_time: 120,
			UseBSON:             false,
			Queue_save_size:     100000,
			Queue_delete_size:   100000,
			Queue_read_size:     100000,
			Queue_add_size:      100000,
			AsfsConfig: types.ASFS_config{
				Enable:        true,
				Max_cpu_usage: 70,
			},
			AsqsConfig: types.ASQS_config{ //
				Enable:                  true,
				Interval:                5,
				Queue_threshold:         1000,
				Worker_count_multiplier: 1,
			},
		}

		// Konwertowanie struktury do JSON
		configData, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			fmt.Printf("Błąd podczas serializacji konfiguracji: %v\n", err)
			return
		}

		// Tworzenie pliku i zapisanie do niego konfiguracji
		err = os.WriteFile(configFileName, configData, 0644)
		if err != nil {
			fmt.Printf("Błąd podczas tworzenia pliku config.json: %v\n", err)
			return
		}

		fmt.Println("Config file sucesfully created...")

		server.StartServer(config)

	} else {
		// Plik istnieje
		fmt.Println("Found config file")

		// Wczytywanie danych z pliku
		data, err := os.ReadFile(configFileName)
		if err != nil {
			fmt.Printf("Błąd podczas odczytu pliku config.json: %v\n", err)
			return
		}

		// Deserializacja danych JSON do struktury Config
		var config types.Config
		err = json.Unmarshal(data, &config)
		if err != nil {
			fmt.Printf("Błąd podczas deserializacji danych z config.json: %v\n", err)
			return
		}

		server.StartServer(config)
	}
}
