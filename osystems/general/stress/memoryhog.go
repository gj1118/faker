package stress

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gj1118/faker/models"
)

const chunkSizeMB = 10 // allocate in 10MB chunks

type MemoryStressor struct {
	mu     sync.Mutex
	chunks [][]byte
	active bool
}

func NewMemoryStressor() *MemoryStressor {
	return &MemoryStressor{}
}

func (m *MemoryStressor) GenerateLoadOnMemory(ctx context.Context, memModel models.MemoryLoadConfig) {
	targetMB := memModel.TargetMB
	if targetMB <= 0 {
		fmt.Println("MemoryLoadConfig.TargetMB must be > 0, skipping memory stress")
		return
	}

	fmt.Printf("Allocating %d MB of memory...\n", targetMB)

	m.mu.Lock()
	m.active = true
	m.mu.Unlock()

	chunkSize := chunkSizeMB * 1024 * 1024
	totalChunks := (targetMB * 1024 * 1024) / chunkSize

	for i := range totalChunks {
		select {
		case <-ctx.Done():
			fmt.Println("Memory stress cancelled during allocation")
			return
		default:
		}

		buf := make([]byte, chunkSize)
		for j := range buf {
			buf[j] = byte(i + j)
		}

		m.mu.Lock()
		m.chunks = append(m.chunks, buf)
		m.mu.Unlock()

		time.Sleep(20 * time.Millisecond) // ramp up gradually instead of a huge instant spike
	}

	fmt.Println("Target memory allocated. Holding...")

	// Periodically touch the memory to prevent any OS-level tricks
	// (e.g. swap compression) from reclaiming it silently.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			for _, chunk := range m.chunks {
				if len(chunk) > 0 {
					chunk[0]++
				}
			}
			m.mu.Unlock()
		}
	}
}

// Clear releases all held memory back to the Go runtime
func (m *MemoryStressor) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.active && len(m.chunks) == 0 {
		return
	}

	fmt.Println("Clearing stress-allocated memory...")
	m.chunks = nil
	m.active = false

	runtime.GC()
	debug.FreeOSMemory()

	fmt.Println("Memory cleared.")
}
