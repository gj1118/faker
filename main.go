package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
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

// desktopPath returns the current user's desktop directory across platforms.
func desktopPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "windows":
		// Respect a custom desktop location set via the registry / shell folder env.
		if p := os.Getenv("USERPROFILE"); p != "" {
			return filepath.Join(p, "Desktop"), nil
		}
		return filepath.Join(home, "Desktop"), nil
	default:
		// macOS and Linux both use ~/Desktop by convention.
		return filepath.Join(home, "Desktop"), nil
	}
}

// chromeDefaultPaths returns (executablePath, profileDir) for the current OS.
func chromeDefaultPaths() (exePath, profileDir string) {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		candidate := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(candidate); err == nil {
			exePath = candidate
		}
		profileDir = filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default")
	case "windows":
		// Try standard Program Files locations via environment variables
		for _, base := range []string{
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("LOCALAPPDATA"),
		} {
			if base == "" {
				continue
			}
			candidate := filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe")
			if _, err := os.Stat(candidate); err == nil {
				exePath = candidate
				break
			}
		}
		profileDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data", "Default")
	default:
		// Linux fallback
		for _, p := range []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium-browser",
		} {
			if _, err := os.Stat(p); err == nil {
				exePath = p
				break
			}
		}
		profileDir = filepath.Join(home, ".config", "google-chrome", "Default")
	}
	return
}

// chromeExePath resolves the Chrome executable path, using config override or auto-detection.
func chromeExePath(cfg ChromeConfig) string {
	if cfg.ExePath != "" {
		return cfg.ExePath
	}
	exePath, _ := chromeDefaultPaths()
	return exePath
}

// chromeProfileDir resolves the profile directory, using config override or auto-detection.
func chromeProfileDir(cfg ChromeConfig) string {
	if cfg.ProfileDir != "" {
		return cfg.ProfileDir
	}
	_, profileDir := chromeDefaultPaths()
	return profileDir
}

// isChromeRunning returns true if a Chrome process is currently running.
func isChromeRunning() bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pgrep", "-x", "Google Chrome")
	case "windows":
		cmd = exec.Command("tasklist", "/FI", "IMAGENAME eq chrome.exe", "/NH")
	default:
		cmd = exec.Command("pgrep", "-x", "chrome")
	}
	out, _ := cmd.Output()
	if runtime.GOOS == "windows" {
		return strings.Contains(strings.ToLower(string(out)), "chrome.exe")
	}
	return strings.TrimSpace(string(out)) != ""
}

// waitForChromeToClose blocks until Chrome is no longer running,
// prompting the user to close it first.
func waitForChromeToClose() {
	if !isChromeRunning() {
		return
	}
	fmt.Println()
	fmt.Println("WARNING: Google Chrome is currently running.")
	fmt.Println("Please close Chrome completely, then press Enter to continue (or Ctrl+C to abort).")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		scanner.Scan()
		if !isChromeRunning() {
			fmt.Println("Chrome is now closed. Proceeding...")
			return
		}
		fmt.Println("Chrome is still running. Please close it and press Enter again.")
	}
}

// toWebKitTime converts a Go time to Chrome's WebKit timestamp format
// (microseconds elapsed since Jan 1, 1601).
func toWebKitTime(t time.Time) int64 {
	const epochDelta = 11644473600 * 1_000_000 // microseconds
	return t.UnixMicro() + epochDelta
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

var trackerDomains = []string{
	"doubleclick.net", "google-analytics.com", "facebook.com", "ads.twitter.com",
	"scorecardresearch.com", "quantserve.com", "adnxs.com", "rubiconproject.com",
	"pubmatic.com", "openx.net", "adsrvr.org", "casalemedia.com",
	"advertising.com", "amazon-adsystem.com", "criteo.com", "bing.com",
	"taboola.com", "outbrain.com", "spotxchange.com", "sharethrough.com",
	"moatads.com", "chartbeat.com", "newrelic.com", "mixpanel.com",
	"segment.io", "hotjar.com", "optimizely.com", "mopub.com",
	"rlcdn.com", "demdex.net", "bluekai.com", "krxd.net",
}

var cookieNames = []string{
	"_ga", "_gid", "_fbp", "_gcl_au", "IDE", "ANID", "NID",
	"SID", "SSID", "APISID", "SAPISID", "uid", "uuid", "visitor_id",
	"tracking_id", "sess_id", "ad_id", "cid", "c_user", "xs",
	"fr", "datr", "spin", "wd", "act", "presence",
}

var tempFilePatterns = []string{
	"tmp_track_%d.dat", "cache_%d.bin", "sess_%d.tmp", "ad_cache_%d.tmp",
	"pixel_%d.gif.tmp", "beacon_%d.dat", "sync_%d.tmp", "uid_%d.dat",
}

var historyURLs = []string{
	"https://www.googleadservices.com/pagead/aclk?sa=L&ai=",
	"https://pixel.facebook.com/tr?id=",
	"https://sync.criteo.com/sync?p=",
	"https://cm.g.doubleclick.net/pixel?google_nid=",
	"https://ib.adnxs.com/getuid?",
	"https://sync.rubiconproject.com/usync?p=",
	"https://x.bidswitch.net/sync?ssp=",
	"https://match.adsrvr.org/track/cmf/generic?ttd_pid=",
	"https://ups.analytics.yahoo.com/ups/",
	"https://s.amazon-adsystem.com/iu3?pid=",
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func randomTimestamp() string {
	t := time.Now().Add(-time.Duration(rand.Intn(30*24)) * time.Hour)
	return t.Format("2006-01-02T15:04:05Z")
}

func pickDomain() string { return trackerDomains[rand.Intn(len(trackerDomains))] }

var loremWords = strings.Fields(
	"lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt" +
		" ut labore et dolore magna aliqua ut enim ad minim veniam quis nostrud exercitation ullamco" +
		" laboris nisi ut aliquip ex ea commodo consequat duis aute irure dolor in reprehenderit in" +
		" voluptate velit esse cillum dolore eu fugiat nulla pariatur excepteur sint occaecat cupidatat" +
		" non proident sunt in culpa qui officia deserunt mollit anim id est laborum")

func loremSentence(words int) string {
	parts := make([]string, words)
	for i := range parts {
		w := loremWords[rand.Intn(len(loremWords))]
		if i == 0 {
			w = strings.ToUpper(w[:1]) + w[1:]
		}
		parts[i] = w
	}
	return strings.Join(parts, " ") + "."
}

func loremParagraph() string {
	sentences := rand.Intn(5) + 3
	out := make([]string, sentences)
	for i := range out {
		out[i] = loremSentence(rand.Intn(12) + 5)
	}
	return strings.Join(out, " ")
}

func loremDocument() string {
	paras := rand.Intn(6) + 3
	out := make([]string, paras)
	for i := range out {
		out[i] = loremParagraph()
	}
	return strings.Join(out, "\n\n")
}

var trashExts = []string{".txt", ".log", ".tmp", ".bak", ".doc", ".csv"}

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
				ext := trashExts[rand.Intn(len(trashExts))]
				name := fmt.Sprintf("deleted_%s_%d%s", randomString(8), idx, ext)
				fpath := filepath.Join(destDir, name)
				if werr := os.WriteFile(fpath, []byte(loremDocument()), 0644); werr != nil {
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
		printTrashProgress("Writing files", inserted, count)
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
				printTrashProgress("Moving to trash", deleted, len(paths))
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

func printTrashProgress(label string, done, total int) {
	if total == 0 {
		return
	}
	const width = 30
	filled := done * width / total
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	fmt.Printf("\r    %-25s [%s] %d/%d", label, bar, done, total)
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
		creationUtc := toWebKitTime(now.Add(-time.Duration(rand.Intn(30*24)) * time.Hour))
		expiresUtc := toWebKitTime(now.Add(time.Duration(rand.Intn(365*24)) * time.Hour))
		_, err := db.Exec(`INSERT OR IGNORE INTO cookies
			(creation_utc, host_key, top_frame_site_key, name, value, encrypted_value,
			 path, expires_utc, is_secure, is_httponly, last_access_utc,
			 has_expires, is_persistent, priority, samesite, source_scheme, source_port, last_update_utc)
			VALUES (?, ?, '', ?, ?, '', '/', ?, ?, ?, ?, 1, 1, 1, -1, 2, 443, ?)`,
			creationUtc,
			"."+pickDomain(),
			cookieNames[rand.Intn(len(cookieNames))],
			randomString(32),
			expiresUtc,
			rand.Intn(2),
			rand.Intn(2),
			creationUtc,
			toWebKitTime(now),
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
			randomString(16), pickDomain(), randomString(64),
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
		base := historyURLs[rand.Intn(len(historyURLs))]
		url := fmt.Sprintf("%s%s&t=%d", base, randomString(12), rand.Intn(99999))
		visitTime := toWebKitTime(now.Add(-time.Duration(rand.Intn(30*24)) * time.Hour))

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
	desktopDir, err := desktopPath()
	if err != nil {
		return 0, fmt.Errorf("resolve desktop: %w", err)
	}
	dir := filepath.Join(desktopDir, "fake_tracker_test", "Temp")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	for i := range count {
		pattern := tempFilePatterns[rand.Intn(len(tempFilePatterns))]
		content := fmt.Sprintf(
			"tracker_origin=%s\nsession_id=%s\nuid=%s\ntimestamp=%s\npayload=%s\n",
			pickDomain(), randomString(24), randomString(16), randomTimestamp(), randomString(128),
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
		fmt.Fprintf(f, "[HKEY_CURRENT_USER\\Software\\Microsoft\\Internet Explorer\\LowRegistry\\DOMStorage\\%s]\n", pickDomain())
		fmt.Fprintf(f, "\"tracking_id\"=\"%s\"\n", randomString(32))
		fmt.Fprintf(f, "\"last_seen\"=\"%s\"\n", randomTimestamp())
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
		name := fmt.Sprintf("shred_%s_%d%s", randomString(8), i, ext)
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

// simple , yet holistic task runner, lol
func run(label string, enabled bool, count int, fn func(string, int) (int, error), baseDir string) int {
	if !enabled {
		fmt.Printf("  %-40s skipped (disabled in config)\n", label)
		return 0
	}
	fmt.Printf("  %-40s ", label)
	n, err := fn(baseDir, count)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return 0
	}
	fmt.Printf("done (%d)\n", n)
	return n
}

func main() {
	fmt.Println("---Faker init---")
	fmt.Println("Will setup your TEST system with fake/bad data, so your security solutions might get to work, otherwise what work do they do ? ;) ")
	fmt.Println("Author - Gagan Janjua")
	fmt.Println("---Faker deinit---")

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

	fmt.Println("=== Faker Entry Generator ===")
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
	waitForChromeToClose()

	total := 0
	total += run("Chrome cookies (SQLite)", cfg.Cookies.Enabled, cfg.Cookies.Count, generateChromeCookies, profileDir)
	total += run("Chrome cache files", cfg.Cache.Enabled, cfg.Cache.Count, generateChromeCache, profileDir)
	total += run("Chrome history (SQLite)", cfg.History.Enabled, cfg.History.Count, generateChromeHistory, profileDir)
	total += run("Temp / junk files", cfg.TempFiles.Enabled, cfg.TempFiles.Count, generateTempFiles, cfg.Output.BaseDir)
	total += run("Registry tracker entries", cfg.Registry.Enabled, cfg.Registry.Count, generateRegistryEntries, cfg.Output.BaseDir)
	total += run("Trash / Recycle Bin files", cfg.Trash.Enabled, cfg.Trash.Count, generateTrashFiles, "")
	total += run("Shredder temp files", cfg.Shredder.Enabled, cfg.Shredder.TempFiles.Count, generateShredderTempFiles, cfg.Output.BaseDir)
	total += run("Firewall traffic generation", cfg.Firewall.Enabled, cfg.Firewall.CallTimes, generateFirewallTraffic, "")

	fmt.Printf("\n✓ Total entries / files generated: %d\n", total)
	fmt.Printf("✓ Chrome data written to: %s\n", profileDir)
	fmt.Printf("✓ Other files written to: %s/fake_tracker_test/\n", cfg.Output.BaseDir)
}
