package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"

	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gj1118/faker/constants"
	"github.com/gj1118/faker/helpers"
	"github.com/gj1118/faker/loggers"
	"github.com/gj1118/faker/models"
	"github.com/gj1118/faker/osystems/general/stress"
	"github.com/gj1118/faker/support"
	_ "modernc.org/sqlite"
)

var firewallSites []string
var cfgPath = "config.toml"
var config models.Config

// chromeExePath resolves the Chrome executable path, using config override or auto-detection.
func chromeExePath(cfg models.ChromeConfig) string {
	if cfg.ExePath != "" {
		return cfg.ExePath
	}
	exePath, _ := helpers.ChromeDefaultPaths()
	return exePath
}

// chromeProfileDir resolves the profile directory, using config override or auto-detection.
func chromeProfileDir(cfg models.ChromeConfig) string {
	if cfg.ProfileDir != "" {
		return cfg.ProfileDir
	}
	_, profileDir := helpers.ChromeDefaultPaths()
	return profileDir
}

func loadConfig(path string) (models.Config, error) {
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return config, err
	}
	desktopDir, err := helpers.GetFakerDirectoryOnDesktop()
	if err != nil {
		log.Fatalf("There was an erro while getting the faker directory on user's desktop. Please see the error → %s\n", err)
	}
	config.Output.BaseDir = desktopDir
	firewallSites = config.Firewall.Sites
	return config, nil
}

func generateTrashFiles(count int) (int, error) {
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

	// write files
	type writeResult struct {
		path string
		err  error
	}
	const writeWorkers = 8
	jobs := make(chan int, writeWorkers)
	results := make(chan writeResult, count)

	var wg sync.WaitGroup
	for range writeWorkers {
		wg.Go(func() {
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
		})
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
		helpers.PrintProgress("Writing files", inserted, count)
	}
	fmt.Println()

	if ctx.Err() != nil {
		return inserted, ctx.Err()
	}

	// Move files to recyclebin/trash
	const delWorkers = 8
	delJobs := make(chan string, delWorkers)

	var delWg sync.WaitGroup
	var delFirstErr error
	var delErrMu sync.Mutex
	deleted := 0
	var deletedMu sync.Mutex

	for range delWorkers {
		delWg.Go(func() {
			for f := range delJobs {
				if ctx.Err() != nil {
					return
				}
				if rerr := support.MoveToRecycleBin(f); rerr != nil {
					delErrMu.Lock()
					if delFirstErr == nil {
						delFirstErr = rerr
					}
					delErrMu.Unlock()
					return
				}
				deletedMu.Lock()
				deleted++
				helpers.PrintProgress("Moving to trash", deleted, len(paths))
				deletedMu.Unlock()
			}
		})
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
	slog.Info("Chrome Profile Cookies", "path", dbPath)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		slog.Error("Unable to get the profile cookie dir", "error", err)
		return 0, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		slog.Error("Unable to open DB", "error", err)
		return 0, fmt.Errorf("open Cookies DB: %w", err)
	} else {
		slog.Info("DB was opened successfuly")
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("Failed to close database", "error", err)
			log.Fatalf("Failed to close database: %v\n", err)
		}
	}()

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
		slog.Error("Create cookies table failed", "error", err)
		return 0, fmt.Errorf("create cookies table: %w", err)
	} else {
		slog.Info("Cookies table was created succssfully")
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
			slog.Error("insert cookies failed ", "error", err)
			return inserted, fmt.Errorf("insert cookie: %w", err)
		} else {
			slog.Info("Cookie information was saved in the database")
		}
		inserted++
	}
	slog.Info("Successfully added records into the database", "count", inserted)
	return inserted, nil
}

func generateChromeCache(profileDir string, count int) (int, error) {
	// Chrome simple-cache files live under Cache/Cache_Data/
	dir := filepath.Join(profileDir, "Cache", "Cache_Data")
	slog.Info("Chrome Cache file path", "path", dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Error("An error happened, while generating chrome cache", "directory_creation_error", err)
		return 0, err
	} else {
		slog.Info("Chrome Cache data was successfully generated")
	}

	for i := range count {
		// Simple cache entry names are 16 hex chars
		fpath := filepath.Join(dir, fmt.Sprintf("%016x_%d", rand.Int63(), i))
		slog.Info("Chrome Cache", "file", fpath)
		content := fmt.Sprintf(
			"HTTP/1.1 200 OK\r\nContent-Type: image/gif\r\nSet-Cookie: uid=%s; Domain=.%s; Path=/\r\n\r\nGIF89a fake tracking pixel data %s",
			helpers.RandomString(16), helpers.PickDomain(), helpers.RandomString(64),
		)
		if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
			slog.Error("An error happened while writing file for Chrome cache", "error", err)
			return i, err
		} else {
			slog.Info("Chrome Cache Data file data was successfully written", "file", fpath)
		}
	}
	slog.Info("Successfully returning chrome cache data", "count", count)
	return count, nil
}

func generateChromeHistory(profileDir string, count int) (int, error) {
	dbPath := filepath.Join(profileDir, "History")
	slog.Info("Chrome History", "path", dbPath)

	if err := os.MkdirAll(profileDir, 0755); err != nil {
		slog.Error("An error happened in generating Chrome History data", "error", err)
		return 0, err
	} else {
		slog.Info("Chrome history directory was created successfully", "path", profileDir)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		slog.Error("An error happened while opening the DB file", "database", dbPath)
		return 0, fmt.Errorf("open History DB: %w", err)
	} else {
		slog.Info("Was successfully able to open the DB File")
	}

	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("Failed to close chrome history database", "error", err)
			log.Fatalf("Failed to close database: %v\n", err)
		} else {
			slog.Info("Chrome history database was closed successfully.")
		}
	}()

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
		slog.Error("An error happened while creating urls table", "error", err)
		return 0, fmt.Errorf("create urls table: %w", err)
	} else {
		slog.Info("URLs table was cleared successfully")
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
		slog.Error("Create Visits table could not be created", "error", err)
		return 0, fmt.Errorf("create visits table: %w", err)
	} else {
		slog.Info("Create Visits table was created successfully")
	}

	now := time.Now()
	inserted := 0
	for range count {
		base := constants.HistoryURLs[rand.Intn(len(constants.HistoryURLs))]
		slog.Info("Chrome History Base URL", "base", base)

		url := fmt.Sprintf("%s%s&t=%d", base, helpers.RandomString(12), rand.Intn(99999))
		slog.Info("Chrome History URL", "url", url)
		visitTime := helpers.ToWebKitTime(now.Add(-time.Duration(rand.Intn(30*24)) * time.Hour))

		res, err := db.Exec(`INSERT INTO urls (url, title, visit_count, typed_count, last_visit_time, hidden)
			VALUES (?, 'Tracking Request', ?, 0, ?, 0)`,
			url, rand.Intn(10)+1, visitTime)
		if err != nil {
			slog.Error("An error happened while inserting url", "error", err)
			return inserted, fmt.Errorf("insert url: %w", err)
		} else {
			slog.Info("Inserting url was successful")
		}
		urlID, _ := res.LastInsertId()

		_, err = db.Exec(`INSERT INTO visits (url, visit_time, from_visit, transition, segment_id, visit_duration)
			VALUES (?, ?, 0, 0, 0, ?)`,
			urlID, visitTime, rand.Intn(10000)*1000)
		if err != nil {
			slog.Error("Could not insert visits data", "error", err)
			return inserted, fmt.Errorf("insert visit: %w", err)
		} else {
			slog.Info("Inserted visited record data successfully")
		}
		inserted++
	}
	slog.Info("Chrome History was done successfully", "records", inserted)
	return inserted, nil
}

func generateFirewallTraffic(callTimes int) (int, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	n := 0
	slog.Info("Firewall Sites", "sites", firewallSites)
	for _, site := range firewallSites {
		for i := range callTimes {
			resp, err := client.Get("http://" + site)
			if err != nil {
				fmt.Printf("    [firewall] %s (attempt %d): %v\n", site, i+1, err)
				continue
			}
			err = resp.Body.Close()
			if err != nil {
				slog.Error("An error occured while generating firewall traffic", "error", err)
				log.Fatalf("An error occured while generating firewall traffic : %v\n", err)
			} else {
				slog.Info("No issues with firewall traffic")
			}
			n++
		}
	}
	slog.Info("Total firewall attempts ", "firewall_attempts", n)
	return n, nil
}

func executeVirus(_ int) (int, error) {
	downloadedPath, err := helpers.DownloadEicar()
	if err != nil {
		slog.Error("There was an error while downloading the virus.", "error", err)
		log.Fatalf("There was an error while downloading the virus. The error is → %s\n", err)
	} else {
		slog.Info("executeVirus - The virus was downloaded succesfullly")
	}

	slog.Info("Downloaded Virus Path", "Path", downloadedPath)

	tempDirectory := path.Join(os.TempDir(), fmt.Sprintf("eicar-%s", helpers.RandomString(10)))
	slog.Info("TempDirectory Path", "Path", tempDirectory)

	err = os.Mkdir(tempDirectory, 0755)
	if err != nil {
		slog.Error("There was an error while creating the temp directory.", "error", err)
		log.Fatalf("There was an error while creating the temp directory. The error is → %s\n", err)
	} else {
		slog.Info("executeVirus - The temp directory was created succesfullly")
	}
	exists, err := helpers.Exists(tempDirectory)

	if err != nil {
		slog.Error("There was an error while checking for tempdir", "error", err)
		log.Fatal("There was an error while checking for tempdir. Please try again later")
	} else {
		slog.Info("Tempdir was obtained successfully", "exists", exists)
	}

	if exists == false {
		slog.Error("tempdir does not exist", "tempdir_path", tempDirectory)
		log.Fatal("TempDir does not exist")
	} else {
		slog.Info("tempdir exists", "tempdir_path", tempDirectory)
	}

	slog.Info("Going to extract and run the Virus")
	helpers.ExtractAndRunEicar(downloadedPath, tempDirectory, config.Virus)
	slog.Info("Executed successfully the  extract and run the Virus feature")

	return 0, nil
}

func generateTempFiles(count int) (int, error) {
	tempFilesDir := filepath.Join(config.Output.BaseDir, "Tracker_remover_tempFiles")
	slog.Info("Generateed TempFile Dir", "tempFilesDir", tempFilesDir)
	if err := os.MkdirAll(tempFilesDir, 0755); err != nil {
		slog.Error("An error happened while creating the tempDirFile", "error", err)
		return 0, err
	} else {
		slog.Info("TempFileDir was generated successfully")
	}

	for i := range count {
		pattern := constants.TempFilePatterns[rand.Intn(len(constants.TempFilePatterns))]
		content := fmt.Sprintf(
			"tracker_origin=%s\nsession_id=%s\nuid=%s\ntimestamp=%s\npayload=%s\n",
			helpers.PickDomain(), helpers.RandomString(24), helpers.RandomString(16), helpers.RandomTimestamp(), helpers.RandomString(128),
		)
		if err := os.WriteFile(filepath.Join(tempFilesDir, fmt.Sprintf(pattern, i)), []byte(content), 0644); err != nil {
			slog.Error("An error was encountered while writing file", "error", err)
			return i, err
		} else {
			generatedFilePath := filepath.Join(tempFilesDir, fmt.Sprintf(pattern, i))
			slog.Info("Successfully wrote the file", "fileinfo", generatedFilePath)
		}
	}
	slog.Info("Generated temp files", "count", count)
	return count, nil
}

func generateRegistryEntries(count int) (int, error) {
	dir := filepath.Join(config.Output.BaseDir, "Registry")
	slog.Info("Registry Folder", "registry", dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Error("There was an error creating regitry folder", "error", err)
		return 0, err
	} else {
		slog.Info("Registry folder was created successfully.", "path", dir)
	}
	registryFilePath := filepath.Join(dir, "fake_registry_trackers.reg")
	f, err := os.Create(registryFilePath)
	if err != nil {
		slog.Error("Error creating registry file.", "error", err)
		return 0, err
	} else {
		slog.Info("Successfully created the registry file.", "path", registryFilePath)
	}

	defer func() {
		if err := f.Close(); err != nil {
			slog.Error("An error was encountered, while closing the registry file", "error", err)
			log.Fatalf("An error happened while writing registry file entries. Error → %v", err)
		} else {
			slog.Info("No errors were encountered while closing the regisstry file")
		}
	}()

	_, _ = fmt.Fprintln(f, "Windows Registry Editor Version 5.00")
	_, _ = fmt.Fprintln(f)

	for range count {
		_, _ = fmt.Fprintf(f, "[HKEY_CURRENT_USER\\Software\\Microsoft\\Internet Explorer\\LowRegistry\\DOMStorage\\%s]\n", helpers.PickDomain())
		_, _ = fmt.Fprintf(f, "\"tracking_id\"=\"%s\"\n", helpers.RandomString(32))
		_, _ = fmt.Fprintf(f, "\"last_seen\"=\"%s\"\n", helpers.RandomTimestamp())
		_, _ = fmt.Fprintf(f, "\"visit_count\"=dword:%08x\n\n", rand.Intn(9999))
	}

	slog.Info("Registry File final", "count", count)
	return count, nil
}

func generateShredderTempFiles(count int) (int, error) {
	dir := filepath.Join(config.Output.BaseDir, "Shredder")
	slog.Info("Shredder file path", "path", dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Error("Could not create the shredder file path", "error", err)
		return 0, err
	} else {
		slog.Info("Successfully created the file shredder path")
	}

	exts := []string{".tmp", ".dat", ".bin", ".bak", ".log"}

	for i := range count {
		size := constants.ShredderFileSizes[rand.Intn(len(constants.ShredderFileSizes))]
		slog.Info("File size", "shredderfilesize", size)
		ext := exts[rand.Intn(len(exts))]
		slog.Info("File extension", "shredderfileextension", ext)
		name := fmt.Sprintf("shred_%s_%d%s", helpers.RandomString(8), i, ext)
		slog.Info("File Name", "shredderfilename", name)
		fpath := filepath.Join(dir, name)
		slog.Info("File Path", "shredderfilepath", fpath)

		f, err := os.Create(fpath)
		if err != nil {
			slog.Error("An error encountered while creating shredder file", "error", err)
			return i, err
		} else {
			slog.Info("Shredder file was created successfully")
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
				_ = f.Close()
				return i, err
			}
			remaining -= n
		}
		if err = f.Close(); err != nil {
			slog.Error("An error occured while closing the shredder files", "error", err)
			log.Fatalf("An error occured while closing the files. Error → %v\n", err)
		} else {
			slog.Info("Successfully closing the files")
		}
	}
	slog.Info("Shredder count", "count", count)
	return count, nil
}

func main() {
	fmt.Println()
	fmt.Println("---Faker init---")
	fmt.Println("Will setup your TEST system with fake/bad data, so your security solutions might get to work, otherwise what work do they do ? ;) ")
	fmt.Println("*************************")
	fmt.Println("Looking for a MacOS or a Linux version - we have a binary for those OSes too. Please don't forget to ask!")
	fmt.Println("*************************")
	fmt.Println("Author - Gagan Janjua")
	fmt.Println("---Faker deinit---")
	fmt.Println()

	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config %q: %v\n", cfgPath, err)
		os.Exit(1)
	}

	fmt.Printf("Config: %s\n\n", cfgPath)

	// init logger -
	loggers.Init(cfg.Log)

	// Detect Chrome installation
	chromeExe := chromeExePath(cfg.Chrome)
	profileDir := chromeProfileDir(cfg.Chrome)

	// Verify Chrome is installed
	if _, err := os.Stat(profileDir); os.IsNotExist(err) {
		slog.Error("Chrome profile directory not found at", "Location", profileDir)
		log.Fatalf("Install Google Chrome or set [chrome] profile_dir in %s\n", cfgPath)
	}
	if chromeExe == "" {
		slog.Error("Google Chrome executable not found on this system")
		log.Fatalf("Install Google Chrome or set [chrome] exe_path in %s\n", cfgPath)
	}

	fmt.Printf("Chrome executable: %s\n", chromeExe)
	slog.Info("Chrome executable", "Path", chromeExe)

	fmt.Printf("Chrome profile:    %s\n\n", profileDir)
	slog.Info("Chrome User Profile ", "Profile Path", profileDir)

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
	total += helpers.RunWithoutBaseDir("Tracker Remover Temp files", cfg.TempFiles.Enabled, cfg.TempFiles.Count, generateTempFiles)
	total += helpers.RunWithoutBaseDir("Registry tracker entries", cfg.Registry.Enabled, cfg.Registry.Count, generateRegistryEntries)
	total += helpers.RunWithoutBaseDir("Trash / Recycle Bin files", cfg.Trash.Enabled, cfg.Trash.Count, generateTrashFiles)
	total += helpers.RunWithoutBaseDir("Shredder temp files", cfg.Shredder.Enabled, cfg.Shredder.TempFiles.Count, generateShredderTempFiles)
	total += helpers.RunWithoutBaseDir("Firewall traffic generation", cfg.Firewall.Enabled, cfg.Firewall.CallTimes, generateFirewallTraffic)
	total += helpers.RunWithoutBaseDir("Virus related asks ", cfg.Virus.Enabled, 0, executeVirus)

	fmt.Printf("\n✓ Total entries / files generated: %d\n", total)
	fmt.Printf("✓ Chrome data written to: %s\n", profileDir)
	result := filepath.Join(cfg.Output.BaseDir, "fake_tracker_test")
	fmt.Printf("✓ Other files written to: %s\n", result)

	runner := stress.NewRunner(cfg)
	runner.Start()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	runner.Stop()

}
