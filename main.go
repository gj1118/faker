package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gj1118/faker/constants"
	"github.com/gj1118/faker/helpers"
	_ "modernc.org/sqlite"
)

var firewallSites []string

type ChromeConfig struct {
	ExePath    string `toml:"exe_path"`
	ProfileDir string `toml:"profile_dir"`
}

type Config struct {
	Output    OutputConfig   `toml:"output"`
	Chrome    ChromeConfig   `toml:"chrome"`
	Cookies   SectionConfig  `toml:"cookies"`
	Cache     SectionConfig  `toml:"cache"`
	History   SectionConfig  `toml:"history"`
	TempFiles SectionConfig  `toml:"temp_files"`
	Registry  SectionConfig  `toml:"registry"`
	Trash     SectionConfig  `toml:"trash"`
	Shredder  ShredderConfig `toml:"shredder"`
	Firewall  FirewallConfig `toml:"firewall"`
}

type FirewallConfig struct {
	Enabled   bool     `toml:"enabled"`
	Sites     []string `toml:"sites"`
	CallTimes int      `toml:"call_times"`
}

type OutputConfig struct {
	BaseDir string `toml:"base_dir"`
}

type SectionConfig struct {
	Enabled bool `toml:"enabled"`
	Count   int  `toml:"count"`
}

type ShredderTempFilesConfig struct {
	Count int `toml:"count"`
}

type ShredderConfig struct {
	Enabled   bool                    `toml:"enabled"`
	TempFiles ShredderTempFilesConfig `toml:"tempfiles"`
}

// chromeExePath resolves the Chrome executable path, using config override or auto-detection.
func chromeExePath(cfg ChromeConfig) string {
	if cfg.ExePath != "" {
		return cfg.ExePath
	}
	exePath, _ := helpers.ChromeDefaultPaths()
	return exePath
}

// chromeProfileDir resolves the profile directory, using config override or auto-detection.
func chromeProfileDir(cfg ChromeConfig) string {
	if cfg.ProfileDir != "" {
		return cfg.ProfileDir
	}
	_, profileDir := helpers.ChromeDefaultPaths()
	return profileDir
}

func loadConfig(path string) (Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Output.BaseDir == "" || cfg.Output.BaseDir == "." {
		wd, err := os.Getwd()
		if err != nil {
			return cfg, err
		}
		cfg.Output.BaseDir = wd
	}
	firewallSites = cfg.Firewall.Sites
	return cfg, nil
}

func generateTrashFiles(_ string, count int) (int, error) {
	// Write files to a staging directory on all platforms; Phase 2 moves each
	// file to the OS recycle bin / trash using the appropriate native API.
	destDir := filepath.Join(os.TempDir(), "faker_trash_stage")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Catch Ctrl-C / SIGTERM so all goroutines and child processes are cleaned up.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	// --- Phase 1: write files with a worker pool ---
	type writeResult struct {
		path string
		err  error
	}
	const writeWorkers = 8
	jobs := make(chan int, writeWorkers)
	results := make(chan writeResult, count)

	var wg sync.WaitGroup
	for range writeWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				ext := constants.TrashExts[rand.Intn(len(constants.TrashExts))]
				name := fmt.Sprintf("deleted_%s_%d%s", helpers.RandomString(8), idx, ext)
				fpath := filepath.Join(destDir, name)
				if werr := os.WriteFile(fpath, []byte(helpers.LoremDocument()), 0644); werr != nil {
					results <- writeResult{err: werr}
					return
				}
				results <- writeResult{path: fpath}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for i := range count {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	fmt.Println() // separate progress from the run() label
	var paths []string
	inserted := 0
	for res := range results {
		if res.err != nil {
			cancel()
			fmt.Println()
			return inserted, res.err
		}
		inserted++
		paths = append(paths, res.path)
		helpers.PrintTrashProgress("Writing files", inserted, count)
	}
	fmt.Println()

	if ctx.Err() != nil {
		return inserted, ctx.Err()
	}

	// --- Phase 2: move staged files to the OS recycle bin / trash ---
	const delWorkers = 8
	delJobs := make(chan string, delWorkers)

	var delWg sync.WaitGroup
	var delFirstErr error
	var delErrMu sync.Mutex
	deleted := 0
	var deletedMu sync.Mutex

	for range delWorkers {
		delWg.Add(1)
		go func() {
			defer delWg.Done()
			for f := range delJobs {
				if ctx.Err() != nil {
					return
				}
				if rerr := moveToRecycleBin(f); rerr != nil {
					delErrMu.Lock()
					if delFirstErr == nil {
						delFirstErr = rerr
					}
					delErrMu.Unlock()
					return
				}
				deletedMu.Lock()
				deleted++
				helpers.PrintTrashProgress("Moving to trash", deleted, len(paths))
				deletedMu.Unlock()
			}
		}()
	}

	go func() {
		defer close(delJobs)
		for _, f := range paths {
			select {
			case delJobs <- f:
			case <-ctx.Done():
				return
			}
		}
	}()

	delWg.Wait()
	fmt.Println()

	if delFirstErr != nil {
		return 0, delFirstErr
	}
	_ = os.Remove(destDir) // remove the now-empty staging dir

	return inserted, nil
}

func generateChromeCookies(profileDir string, count int) (int, error) {
	dbPath := filepath.Join(profileDir, "Cookies")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return 0, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, fmt.Errorf("open Cookies DB: %w", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS cookies (
		creation_utc       INTEGER NOT NULL,
		host_key           TEXT NOT NULL,
		top_frame_site_key TEXT NOT NULL DEFAULT '',
		name               TEXT NOT NULL,
		value              TEXT NOT NULL DEFAULT '',
		encrypted_value    BLOB DEFAULT '',
		path               TEXT NOT NULL,
		expires_utc        INTEGER NOT NULL,
		is_secure          INTEGER NOT NULL,
		is_httponly        INTEGER NOT NULL,
		last_access_utc    INTEGER NOT NULL,
		has_expires        INTEGER NOT NULL DEFAULT 1,
		is_persistent      INTEGER NOT NULL DEFAULT 1,
		priority           INTEGER NOT NULL DEFAULT 1,
		samesite           INTEGER NOT NULL DEFAULT -1,
		source_scheme      INTEGER NOT NULL DEFAULT 0,
		source_port        INTEGER NOT NULL DEFAULT -1,
		last_update_utc    INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return 0, fmt.Errorf("create cookies table: %w", err)
	}

	now := time.Now()
	inserted := 0
	for range count {
		creationUtc := helpers.ToWebKitTime(now.Add(-time.Duration(rand.Intn(30*24)) * time.Hour))
		expiresUtc := helpers.ToWebKitTime(now.Add(time.Duration(rand.Intn(365*24)) * time.Hour))
		_, err := db.Exec(`INSERT OR IGNORE INTO cookies
			(creation_utc, host_key, top_frame_site_key, name, value, encrypted_value,
			 path, expires_utc, is_secure, is_httponly, last_access_utc,
			 has_expires, is_persistent, priority, samesite, source_scheme, source_port, last_update_utc)
			VALUES (?, ?, '', ?, ?, '', '/', ?, ?, ?, ?, 1, 1, 1, -1, 2, 443, ?)`,
			creationUtc,
			"."+helpers.PickDomain(),
			constants.CookieNames[rand.Intn(len(constants.CookieNames))],
			helpers.RandomString(32),
			expiresUtc,
			rand.Intn(2),
			rand.Intn(2),
			creationUtc,
			helpers.ToWebKitTime(now),
		)
		if err != nil {
			return inserted, fmt.Errorf("insert cookie: %w", err)
		}
		inserted++
	}
	return inserted, nil
}

func generateChromeCache(profileDir string, count int) (int, error) {
	// Chrome simple-cache files live under Cache/Cache_Data/
	dir := filepath.Join(profileDir, "Cache", "Cache_Data")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	for i := range count {
		// Simple cache entry names are 16 hex chars
		fpath := filepath.Join(dir, fmt.Sprintf("%016x_%d", rand.Int63(), i))
		content := fmt.Sprintf(
			"HTTP/1.1 200 OK\r\nContent-Type: image/gif\r\nSet-Cookie: uid=%s; Domain=.%s; Path=/\r\n\r\nGIF89a fake tracking pixel data %s",
			helpers.RandomString(16), helpers.PickDomain(), helpers.RandomString(64),
		)
		if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
			return i, err
		}
	}
	return count, nil
}

func generateChromeHistory(profileDir string, count int) (int, error) {
	dbPath := filepath.Join(profileDir, "History")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return 0, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, fmt.Errorf("open History DB: %w", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS urls (
		id              INTEGER PRIMARY KEY,
		url             LONGVARCHAR NOT NULL,
		title           LONGVARCHAR DEFAULT '',
		visit_count     INTEGER DEFAULT 0 NOT NULL,
		typed_count     INTEGER DEFAULT 0 NOT NULL,
		last_visit_time INTEGER NOT NULL,
		hidden          INTEGER DEFAULT 0 NOT NULL
	)`)
	if err != nil {
		return 0, fmt.Errorf("create urls table: %w", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS visits (
		id             INTEGER PRIMARY KEY,
		url            INTEGER NOT NULL,
		visit_time     INTEGER NOT NULL,
		from_visit     INTEGER DEFAULT 0,
		transition     INTEGER DEFAULT 0 NOT NULL,
		segment_id     INTEGER DEFAULT 0,
		visit_duration INTEGER DEFAULT 0 NOT NULL
	)`)
	if err != nil {
		return 0, fmt.Errorf("create visits table: %w", err)
	}

	now := time.Now()
	inserted := 0
	for range count {
		base := constants.HistoryURLs[rand.Intn(len(constants.HistoryURLs))]
		url := fmt.Sprintf("%s%s&t=%d", base, helpers.RandomString(12), rand.Intn(99999))
		visitTime := helpers.ToWebKitTime(now.Add(-time.Duration(rand.Intn(30*24)) * time.Hour))

		res, err := db.Exec(`INSERT INTO urls (url, title, visit_count, typed_count, last_visit_time, hidden)
			VALUES (?, 'Tracking Request', ?, 0, ?, 0)`,
			url, rand.Intn(10)+1, visitTime)
		if err != nil {
			return inserted, fmt.Errorf("insert url: %w", err)
		}
		urlID, _ := res.LastInsertId()

		_, err = db.Exec(`INSERT INTO visits (url, visit_time, from_visit, transition, segment_id, visit_duration)
			VALUES (?, ?, 0, 0, 0, ?)`,
			urlID, visitTime, rand.Intn(10000)*1000)
		if err != nil {
			return inserted, fmt.Errorf("insert visit: %w", err)
		}
		inserted++
	}
	return inserted, nil
}

func generateFirewallTraffic(_ string, callTimes int) (int, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	n := 0
	for _, site := range firewallSites {
		for i := range callTimes {
			resp, err := client.Get("http://" + site)
			if err != nil {
				fmt.Printf("    [firewall] %s (attempt %d): %v\n", site, i+1, err)
				continue
			}
			resp.Body.Close()
			n++
		}
	}
	return n, nil
}

func generateTempFiles(_ string, count int) (int, error) {
	const fakerDir = "fake_tracker_test"

	desktopDir, err := helpers.DesktopPath()
	if err != nil {
		return 0, fmt.Errorf("resolve desktop: %w", err)
	}

	dir := filepath.Join(desktopDir, fakerDir, "Temp")
	dirExists, err := helpers.Exists(dir)

	if err != nil {
		fmt.Println("It seems that there was an error while checking if the directory exists or not. Bailing out. ")
		os.Exit(1)

	}

	switch dirExists {
	case true:
		fmt.Println("Directory already exists. Will create a new dir with similar name")
		dir = filepath.Join(desktopDir, fmt.Sprintf("%s_%s", fakerDir, helpers.RandomString(5)), "Temp")
	case false:
		fmt.Println("Directory does not exist. Go ahead , create a new one")
	}

	// everyting was good, now go ahead and create the dir
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	for i := range count {
		pattern := constants.TempFilePatterns[rand.Intn(len(constants.TempFilePatterns))]
		content := fmt.Sprintf(
			"tracker_origin=%s\nsession_id=%s\nuid=%s\ntimestamp=%s\npayload=%s\n",
			helpers.PickDomain(), helpers.RandomString(24), helpers.RandomString(16), helpers.RandomTimestamp(), helpers.RandomString(128),
		)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf(pattern, i)), []byte(content), 0644); err != nil {
			return i, err
		}
	}
	return count, nil
}

func generateRegistryEntries(baseDir string, count int) (int, error) {
	dir := filepath.Join(baseDir, "fake_tracker_test", "Registry")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	f, err := os.Create(filepath.Join(dir, "fake_registry_trackers.reg"))
	if err != nil {
		return 0, err
	}
	defer f.Close()

	fmt.Fprintln(f, "Windows Registry Editor Version 5.00")
	fmt.Fprintln(f)

	for range count {
		fmt.Fprintf(f, "[HKEY_CURRENT_USER\\Software\\Microsoft\\Internet Explorer\\LowRegistry\\DOMStorage\\%s]\n", helpers.PickDomain())
		fmt.Fprintf(f, "\"tracking_id\"=\"%s\"\n", helpers.RandomString(32))
		fmt.Fprintf(f, "\"last_seen\"=\"%s\"\n", helpers.RandomTimestamp())
		fmt.Fprintf(f, "\"visit_count\"=dword:%08x\n\n", rand.Intn(9999))
	}
	return count, nil
}

// shredderFileSizes defines the pool of file sizes used when generating shredder temp files.
// Files are spread across small (1 KB), medium (64 KB–512 KB), and large (1 MB–10 MB) tiers
// so that the shredder has a realistic variety of workloads to process.
var shredderFileSizes = []int{
	1 * 1024,         // 1 KB
	4 * 1024,         // 4 KB
	16 * 1024,        // 16 KB
	64 * 1024,        // 64 KB
	256 * 1024,       // 256 KB
	512 * 1024,       // 512 KB
	1 * 1024 * 1024,  // 1 MB
	4 * 1024 * 1024,  // 4 MB
	10 * 1024 * 1024, // 10 MB
}

func generateShredderTempFiles(baseDir string, count int) (int, error) {
	dir := filepath.Join(baseDir, "fake_tracker_test", "Shredder")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}

	exts := []string{".tmp", ".dat", ".bin", ".bak", ".log"}

	for i := range count {
		size := shredderFileSizes[rand.Intn(len(shredderFileSizes))]
		ext := exts[rand.Intn(len(exts))]
		name := fmt.Sprintf("shred_%s_%d%s", helpers.RandomString(8), i, ext)
		fpath := filepath.Join(dir, name)

		f, err := os.Create(fpath)
		if err != nil {
			return i, err
		}

		// Write random binary data in 4 KB chunks to avoid large allocations.
		const chunkSize = 4 * 1024
		buf := make([]byte, chunkSize)
		remaining := size
		for remaining > 0 {
			n := min(remaining, chunkSize)
			for j := range n {
				buf[j] = byte(rand.Intn(256))
			}
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return i, err
			}
			remaining -= n
		}
		f.Close()
	}
	return count, nil
}

func main() {
	fmt.Println()
	fmt.Println("---Faker init---")
	fmt.Println("Will setup your TEST system with fake/bad data, so your security solutions might get to work, otherwise what work do they do ? ;) ")
	fmt.Println("Author - Gagan Janjua")
	fmt.Println("---Faker deinit---")
	fmt.Println()
	

	rand.Seed(time.Now().UnixNano())

	cfgPath := "config.toml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config %q: %v\n", cfgPath, err)
		os.Exit(1)
	}

	fmt.Printf("Config: %s\n\n", cfgPath)

	// Detect Chrome installation
	chromeExe := chromeExePath(cfg.Chrome)
	profileDir := chromeProfileDir(cfg.Chrome)

	// Verify Chrome is installed
	if _, err := os.Stat(profileDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "ERROR: Chrome profile directory not found at:\n  %s\n", profileDir)
		fmt.Fprintf(os.Stderr, "Install Google Chrome or set [chrome] profile_dir in %s\n", cfgPath)
		os.Exit(1)
	}
	if chromeExe == "" {
		fmt.Fprintf(os.Stderr, "ERROR: Google Chrome executable not found on this system.\n")
		fmt.Fprintf(os.Stderr, "Install Google Chrome or set [chrome] exe_path in %s\n", cfgPath)
		os.Exit(1)
	}

	fmt.Printf("Chrome executable: %s\n", chromeExe)
	fmt.Printf("Chrome profile:    %s\n\n", profileDir)

	fmt.Print("Proceed and generate fake data? [Y/N]: ")
	var answer string
	_, err = fmt.Scanln(&answer)
	if err != nil {
		log.Fatal(err)
	}
	if answer != "Y" {
		fmt.Println("Aborted.")
		os.Exit(0)
	}

	// Ensure Chrome is closed before touching its databases
	helpers.WaitForChromeToClose()

	total := 0
	total += helpers.Run("Chrome cookies (SQLite)", cfg.Cookies.Enabled, cfg.Cookies.Count, generateChromeCookies, profileDir)
	total += helpers.Run("Chrome cache files", cfg.Cache.Enabled, cfg.Cache.Count, generateChromeCache, profileDir)
	total += helpers.Run("Chrome history (SQLite)", cfg.History.Enabled, cfg.History.Count, generateChromeHistory, profileDir)
	total += helpers.Run("Temp / junk files", cfg.TempFiles.Enabled, cfg.TempFiles.Count, generateTempFiles, cfg.Output.BaseDir)
	total += helpers.Run("Registry tracker entries", cfg.Registry.Enabled, cfg.Registry.Count, generateRegistryEntries, cfg.Output.BaseDir)
	total += helpers.Run("Trash / Recycle Bin files", cfg.Trash.Enabled, cfg.Trash.Count, generateTrashFiles, "")
	total += helpers.Run("Shredder temp files", cfg.Shredder.Enabled, cfg.Shredder.TempFiles.Count, generateShredderTempFiles, cfg.Output.BaseDir)
	total += helpers.Run("Firewall traffic generation", cfg.Firewall.Enabled, cfg.Firewall.CallTimes, generateFirewallTraffic, "")

	fmt.Printf("\n✓ Total entries / files generated: %d\n", total)
	fmt.Printf("✓ Chrome data written to: %s\n", profileDir)
	fmt.Printf("✓ Other files written to: %s/fake_tracker_test/\n", cfg.Output.BaseDir)
}
