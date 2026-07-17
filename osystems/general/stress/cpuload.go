// stress/cpu.go
package stress

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/gj1118/faker/constants"
	"github.com/gj1118/faker/models"
)

func cpuWorker(ctx context.Context, targetPercent int) {
	busy := time.Duration(targetPercent) * 10 * time.Millisecond
	idle := time.Second - busy

	x := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		for time.Since(start) < busy {
			x += rand.Intn(1000) * rand.Intn(1000)
		}

		if idle > 0 {
			time.Sleep(idle)
		}
	}
}

// GenerateLoadOnCPU blocks until ctx is cancelled. Call it in a goroutine.
func GenerateLoadOnCPU(ctx context.Context, cpuModel models.CPULoadConfig) {
	cores := runtime.NumCPU()
	if cpuModel.Cores > runtime.NumCPU() {
		fmt.Println("CPU Cores mentioned in the config is much larger than the cores on this machine. I will use all the cores")
	} else if cpuModel.Cores > 0 {
		cores = cpuModel.Cores
	}
	runtime.GOMAXPROCS(cores)

	load := cpuModel.PercentageLoad
	if load < 0 {
		load = 0
	} else if load > 100 {
		fmt.Println("PercentageLoad > 100 doesn't make sense, clamping to 100")
		load = 100
	}

	if load >= constants.MAX_CPU_USAGE_WARNING {
		fmt.Println("You have set the cpuLoad at a very high level. Are you sure ? This might put your system in an unrecoverable state. Press Y to continue!")
		var answer string
		_, err := fmt.Scanln(&answer)
		if err != nil {
			log.Fatal(err)
		}
		if answer != "Y" {
			fmt.Println("CPU stress aborted")
			return
		}
	}

	fmt.Printf("Putting CPU stress on your machine! Cores: %d, Load: %d%%\n", cores, load)

	var wg sync.WaitGroup
	for i := 0; i < cores; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cpuWorker(ctx, load)
		}()
	}
	wg.Wait() // returns once ctx is cancelled and all workers exit
}
