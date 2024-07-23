package server

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/shirou/gopsutil/v3/process"
)

func (db *Database) autoScaleWorkers() {
	ticker := time.NewTicker(time.Duration(config.AsqsConfig.Interval) * time.Second)
	defer ticker.Stop()

	go func() {
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
		}
	}()

	gui, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		fmt.Println("Failed to create GUI:", err)
		return
	}
	defer gui.Close()

	gui.SetManagerFunc(layout)

	if err := keybindings(gui, db); err != nil {
		fmt.Println("Failed to set key bindings:", err)
		return
	}

	if err := gui.MainLoop(); err != nil && err != gocui.ErrQuit {
		fmt.Println("Failed to start GUI loop:", err)
	}
}

func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()

	// Layout for queues
	if v, err := g.SetView("queues", 0, 0, maxX/2-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Queue Lengths and Workers"
		v.Wrap = true
	}

	// Layout for messages
	if v, err := g.SetView("messages", maxX/2, 0, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Messages"
		v.Wrap = true
		v.Autoscroll = true
	}

	return nil
}

func keybindings(g *gocui.Gui, db *Database) error {
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	go updateView(g, db)
	return nil
}

func updateView(g *gocui.Gui, db *Database) {
	for {
		g.Update(func(g *gocui.Gui) error {
			// Update the queues view
			v, err := g.View("queues")
			if err != nil {
				return err
			}
			v.Clear()

			saveQueueLen := len(db.saveQueue)
			deleteQueueLen := len(db.deleteQueue)
			readQueueLen := len(db.readQueue)
			addQueueLen := len(db.addQueue)

			fmt.Fprintf(v, "Save Queue Length: %d, Workers: %d\n", saveQueueLen, atomic.LoadInt32(&db.saveWorkers))
			fmt.Fprintf(v, "Add Queue Length: %d, Workers: %d\n", addQueueLen, atomic.LoadInt32(&db.addWorkers))
			fmt.Fprintf(v, "Delete Queue Length: %d, Workers: %d\n", deleteQueueLen, atomic.LoadInt32(&db.deleteWorkers))
			fmt.Fprintf(v, "Read Queue Length: %d, Workers: %d\n", readQueueLen, atomic.LoadInt32(&db.readWorkers))

			// Additional info
			fmt.Fprintf(v, "\nStatistics:\n")
			fmt.Fprintf(v, "Finished Save Time: %d\n", db.totalSaveOperations)
			fmt.Fprintf(v, "Finished Read Time: %d\n", db.totalReadOperations)
			fmt.Fprintf(v, "Finished Delete Time: %d\n", db.totalDeleteOperations)
			fmt.Fprintf(v, "Finished Add Time: %d\n", db.totalAddOperations)

			fmt.Fprintf(v, "\nStatistics - 2:\n")
			fmt.Fprintf(v, "Average Save Time: %.2f ms\n", db.avgSaveTime)
			fmt.Fprintf(v, "Average Read Time: %.2f ms\n", db.avgReadTime)
			fmt.Fprintf(v, "Average Delete Time: %.2f ms\n", db.avgDeleteTime)
			fmt.Fprintf(v, "Average Add Time: %.2f ms\n", db.avgAddTime)

			// CPU and Memory Usage for the application
			proc, _ := process.NewProcess(int32(os.Getpid()))
			cpuPercent, _ := proc.CPUPercent()
			memInfo, _ := proc.MemoryInfo()
			memPercent, _ := proc.MemoryPercent()
			memUsedMB := memInfo.RSS / (1024 * 1024)
			totalMemMB := memInfo.VMS / (1024 * 1024)

			fmt.Fprintf(v, "\nSystem Resources:\n")
			fmt.Fprintf(v, "CPU Usage: %.2f%%\n", cpuPercent)
			fmt.Fprintf(v, "Memory Usage: %.2f%% (%v/%v MB)\n", memPercent, memUsedMB, totalMemMB)

			return nil
		})
		g.Update(func(g *gocui.Gui) error {
			// Update the queues view
			v, err := g.View("messages")
			if err != nil {
				return err
			}
			v.Clear()

			for _, msg := range messageBuffer {
				fmt.Fprintf(v, "%s\n", msg)
			}

			return nil
		})
		time.Sleep(1 * time.Second)
	}
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func displayMessage(message string) {
	messageBuffer = append(messageBuffer, message)
}
func DisplayMessage(message string) {
	messageBuffer = append(messageBuffer, message)
}

func Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()

	// Create or update the "queues" view
	if v, err := g.SetView("queues", 0, 0, maxX/2-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Queue Lengths and Workers"
		v.Wrap = true
	}

	// Create or update the "messages" view
	if v, err := g.SetView("messages", maxX/2, 0, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Messages"
		v.Wrap = true
		v.Autoscroll = true
	}

	return nil
}

func Keybindings(g *gocui.Gui, db *Database) error {
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	go updateView(g, db)
	return nil
}
