package server

import (
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

func (db *Database) autoScaleWorkers() {
	ticker := time.NewTicker(time.Duration(config.AsqsConfig.Interval) * time.Second)
	defer ticker.Stop()

	firstRun := true

	for range ticker.C {
		saveQueueLen := len(db.saveQueue)
		deleteQueueLen := len(db.deleteQueue)
		readQueueLen := len(db.readQueue)
		addQueueLen := len(db.addQueue)

		if saveQueueLen > config.AsqsConfig.Queue_threshold {
			go db.processSaveQueue()
			atomic.AddInt32(&db.saveWorkers, 1)
		}
		if deleteQueueLen > config.AsqsConfig.Queue_threshold {
			go db.processDeleteQueue()
			atomic.AddInt32(&db.deleteWorkers, 1)
		}
		if readQueueLen > config.AsqsConfig.Queue_threshold {
			go db.processReadQueue()
			atomic.AddInt32(&db.readWorkers, 1)
		}
		if addQueueLen > config.AsqsConfig.Queue_threshold {
			go db.processAddQueue()
			atomic.AddInt32(&db.addWorkers, 1)
		}

		// Print statistics to the terminal
		if firstRun {
			clearTerminal()
			firstRun = false
		} else {
			moveCursorUp(4) // Zwiększ o 4 linie dla dodania nowej sekcji
		}
		fmt.Printf("Save Queue Length: %d, Workers: %d\n", saveQueueLen, atomic.LoadInt32(&db.saveWorkers))
		fmt.Printf("Add Queue Length: %d, Workers: %d\n", addQueueLen, atomic.LoadInt32(&db.addWorkers))
		fmt.Printf("Delete Queue Length: %d, Workers: %d\n", deleteQueueLen, atomic.LoadInt32(&db.deleteWorkers))
		fmt.Printf("Read Queue Length: %d, Workers: %d\n", readQueueLen, atomic.LoadInt32(&db.readWorkers))
	}
}

// Function to clear the terminal screen
func clearTerminal() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// Function to move the cursor up by n lines
func moveCursorUp(n int) {
	fmt.Printf("\033[%dA", n)
}
