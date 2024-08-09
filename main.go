package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	server "pawiu-db/server"
	types "pawiu-db/types"

	"github.com/jroimartin/gocui"
)

func main() {
	const configFileName = "database_config.json"

	gui, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Fatalf("Failed to create GUI: %v", err)
	}
	defer gui.Close()

	gui.SetManagerFunc(server.Layout)

	if err := server.Keybindings(gui, nil); err != nil {
		log.Fatalf("Failed to set key bindings: %v", err)
	}

	if _, err := os.Stat(configFileName); os.IsNotExist(err) {

		fmt.Println("File config.json dont exist. Creating new file...")

		// Przykładowa konfiguracja
		config := types.Config{
			Port:                5432,
			Tcp_Port:            5431,
			Api_key:             "database_acces_api_key",
			Version:             "1.2.4",
			File_name:           "db",
			Max_keys_per_file:   10000,
			Cache_incoming_all:  true,
			Cache_incoming_time: 120,
			Cache_outgoing_all:  true,
			Cache_outgoing_time: 120,
			UseBSON:             false,
			Queue_save_size:     100000,
			Queue_delete_size:   100000,
			Queue_read_size:     100000,
			Queue_add_size:      100000,
			Max_goroutines:      100,
			Batch_Size:          1000,
			AsfsConfig: types.ASFS_config{
				Enable:        true,
				Max_cpu_usage: 70,
			},
			AsqsConfig: types.ASQS_config{
				Enable:                  true,
				Interval:                5,
				Queue_threshold:         1000,
				Worker_count_multiplier: 1,
			},
		}

		// Konwertowanie struktury do JSON
		configData, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			server.DisplayMessage(fmt.Sprintf("Błąd podczas serializacji konfiguracji: %v\n", err))
			return
		}

		// Tworzenie pliku i zapisanie do niego konfiguracji
		err = os.WriteFile(configFileName, configData, 0644)
		if err != nil {
			server.DisplayMessage(fmt.Sprintf("Błąd podczas tworzenia pliku config.json: %v\n", err))
			return
		}

		server.DisplayMessage(fmt.Sprintf("Config file sucesfully created..."))

		server.StartServer(config, gui)

		if err := gui.MainLoop(); err != nil && err != gocui.ErrQuit {
			log.Fatalf("Failed to start GUI loop: %v", err)
		}

	} else {
		// Plik istnieje
		server.DisplayMessage(fmt.Sprintf("Found config file"))

		// Wczytywanie danych z pliku
		data, err := os.ReadFile(configFileName)
		if err != nil {
			server.DisplayMessage(fmt.Sprintf("Błąd podczas odczytu pliku config.json: %v\n", err))
			return
		}

		// Deserializacja danych JSON do struktury Config
		var config types.Config
		err = json.Unmarshal(data, &config)
		if err != nil {
			server.DisplayMessage(fmt.Sprintf("Błąd podczas deserializacji danych z config.json: %v\n", err))
			return
		}

		server.StartServer(config, gui)

		if err := gui.MainLoop(); err != nil && err != gocui.ErrQuit {
			log.Fatalf("Failed to start GUI loop: %v", err)
		}

	}
}
