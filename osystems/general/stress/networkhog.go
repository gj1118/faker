package stress

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	// Refactor these into config.toml later.

	DefaultTarget = "127.0.0.1:9000"

	DefaultWorkers = 10

	DefaultPayloadSize = 64 * 1024

	DefaultDuration = 60 * time.Second

	ReadBufferSize = 64 * 1024

	ReportInterval = 1 * time.Second

	MaxWorkers = 1000
)

type Config struct {
	Target      string
	Workers     int
	PayloadSize int
	Duration    time.Duration
}

type Stats struct {
	BytesSent uint64
	BytesRecv uint64
	Errors    uint64
}

func StartNetworkStress() {
	cfg := Config{}
	cfg.Target = DefaultTarget
	cfg.Workers = DefaultWorkers
	cfg.PayloadSize = DefaultPayloadSize
	cfg.Duration = DefaultDuration

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	stats := &Stats{}

	stopSignals(cancel)

	payload := make([]byte, cfg.PayloadSize)

	for i := range payload {
		payload[i] = byte(i % 255)
	}

	var wg sync.WaitGroup

	fmt.Println("networkhog starting")
	fmt.Printf("target: %s\n", cfg.Target)
	fmt.Printf("workers: %d\n", cfg.Workers)
	fmt.Printf("duration: %s\n", cfg.Duration)

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()
			worker(ctx, cfg, payload, stats)
		}(i)
	}

	report(ctx, stats)

	wg.Wait()

	fmt.Println()
	fmt.Println("completed")
	printStats(stats)
}

func validateConfig(cfg *Config) {
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers
	}

	if cfg.Workers > MaxWorkers {
		cfg.Workers = MaxWorkers
	}

	if cfg.PayloadSize <= 0 {
		cfg.PayloadSize = DefaultPayloadSize
	}

	if cfg.Duration <= 0 {
		cfg.Duration = DefaultDuration
	}
}

func worker(
	ctx context.Context,
	cfg Config,
	payload []byte,
	stats *Stats,
) {
	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		conn, err := net.DialTimeout(
			"tcp",
			cfg.Target,
			5*time.Second,
		)

		if err != nil {
			atomic.AddUint64(&stats.Errors, 1)
			continue
		}

		handleConnection(
			ctx,
			conn,
			payload,
			stats,
		)

		conn.Close()
	}
}

func handleConnection(
	ctx context.Context,
	conn net.Conn,
	payload []byte,
	stats *Stats,
) {
	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		err := conn.SetWriteDeadline(
			time.Now().Add(5 * time.Second),
		)

		if err != nil {
			atomic.AddUint64(&stats.Errors, 1)
			return
		}

		n, err := conn.Write(payload)

		if err != nil {
			atomic.AddUint64(&stats.Errors, 1)
			return
		}

		atomic.AddUint64(
			&stats.BytesSent,
			uint64(n),
		)

		buf := make([]byte, ReadBufferSize)

		err = conn.SetReadDeadline(
			time.Now().Add(100 * time.Millisecond),
		)

		if err != nil {
			return
		}

		n, err = conn.Read(buf)

		if err != nil && err != io.EOF {
			continue
		}

		if n > 0 {
			atomic.AddUint64(
				&stats.BytesRecv,
				uint64(n),
			)
		}
	}
}

func report(
	ctx context.Context,
	stats *Stats,
) {
	ticker := time.NewTicker(ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			fmt.Printf(
				"sent=%s recv=%s errors=%d\n",
				formatBytes(
					atomic.LoadUint64(&stats.BytesSent),
				),
				formatBytes(
					atomic.LoadUint64(&stats.BytesRecv),
				),
				atomic.LoadUint64(&stats.Errors),
			)
		}
	}
}

func printStats(stats *Stats) {
	fmt.Printf(
		"bytes sent: %s\n",
		formatBytes(
			atomic.LoadUint64(&stats.BytesSent),
		),
	)

	fmt.Printf(
		"bytes recv: %s\n",
		formatBytes(
			atomic.LoadUint64(&stats.BytesRecv),
		),
	)

	fmt.Printf(
		"errors: %d\n",
		atomic.LoadUint64(&stats.Errors),
	)
}

func stopSignals(cancel context.CancelFunc) {
	ch := make(chan os.Signal, 1)

	signal.Notify(
		ch,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	go func() {
		<-ch
		cancel()
	}()
}

func formatBytes(v uint64) string {
	const unit = 1024

	if v < unit {
		return fmt.Sprintf("%d B", v)
	}

	div := uint64(unit)
	exp := 0

	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf(
		"%.2f %ciB",
		float64(v)/float64(div),
		"KMGTPE"[exp],
	)
}
