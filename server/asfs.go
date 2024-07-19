package server

import (
	"pawiu-db/types"

	"fmt"
	"time"

	"github.com/shirou/gopsutil/cpu"
)

// todo: zrobić to na ticker jak w queue żeby działało w pentli (jeżeli jeszcze nie zostało uruchomione),
// trzeba dodać nowe queue w db żeby blokowało reszte queue przed nadpisywaniem danych podczas ich przenoszenia
// albo sklonować część map i przenosic dane za pomocą read(key)

// W db.go dodać obsługę (jeżeli value == %ASFS%<key>%END%)
// + jeżeli w db_fs jest key o takiej wartości jak key z value
// to odczytaj wartość z odpowiedniego pliku

func AutoScalingFileSystem(config types.Config) {
	if !get_cpu_usage(config) {
		return
	}
	// 1. stwórz plik w którym będą key <> = file	(plik db_fs)
	// 2. skopiuj mapę z db
	// 3. przejdź po każdym elemencie mapy i sprawdź czy cos ychodzi poza limit
	// 4. jeżeli tak to stwórz odpowiedni plik i przenieś tam dane
}

func get_cpu_usage(config types.Config) bool {
	// Get CPU usage percentages
	percentages, err := cpu.Percent(time.Second, false)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return false
	}

	// Print the CPU usage
	for _, percentage := range percentages {
		// fmt.Printf("CPU %d usage: %.2f%%\n", i, percentage)
		// TODO: cover > 1cpu server
		// (policzyć średną użycia wszsytkich procków)
		if percentage < float64(config.AsfsConfig.Max_cpu_usage) {
			return true
		}
	}
	return false
}
