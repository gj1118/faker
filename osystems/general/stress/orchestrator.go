package stress

import (
	"context"
	"fmt"
	"sync"

	"github.com/gj1118/faker/models"
)

// Runner owns the lifecycle of whichever stress tests are enabled in Config.
// Create one with NewRunner, call Start(), and Stop() when you're done —
// Stop() cancels everything and clears any memory that was allocated.
type Runner struct {
	cfg    models.Config
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	memStressor *MemoryStressor
}

func NewRunner(cfg models.Config) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		memStressor: NewMemoryStressor(),
	}
}

// Start launches the enabled stress tests. It does not block — call Stop()
// (typically from a signal handler in main) when you want to shut down.
func (r *Runner) Start() {
	if !r.cfg.CpuLoad.Enabled && !r.cfg.MemoryLoad.Enabled{
		fmt.Println("Config has both CPU and Memory stress disabled — nothing to do.")
		return
	}

	fmt.Println("*************************")
	fmt.Println("*************************")
	fmt.Println("*************************")
	fmt.Println("Press Ctrl+c to stop the test")
	fmt.Println("*************************")
	fmt.Println("*************************")
	fmt.Println("*************************")

	if r.cfg.CpuLoad.Enabled{
		r.wg.Go(func() {
			GenerateLoadOnCPU(r.ctx, r.cfg.CpuLoad)
		})
	}

	if r.cfg.MemoryLoad.Enabled{
		r.wg.Go(func() {
			r.memStressor.GenerateLoadOnMemory(r.ctx, r.cfg.MemoryLoad)
		})
	}
}

// Stop cancels all running stress tests, waits for them to exit, and clears
// any memory that was allocated during the run.
func (r *Runner) Stop() {
	fmt.Println("Stopping stress tests...")
	r.cancel()
	r.wg.Wait()

	if r.cfg.MemoryLoad.Enabled {
		r.memStressor.Clear()
	}

	fmt.Println("All stress tests stopped and cleaned up.")
}
