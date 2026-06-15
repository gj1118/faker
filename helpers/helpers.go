package helpers

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gj1118/faker/constants"
	"github.com/gj1118/faker/models"
)

func Exists(path string) (bool, error) {
	slog.Info("Exists:: Will check if the path exists or not", "path", path)
	_, err := os.Stat(path)
	if err == nil {
		slog.Error("Exists:: There as an error while checking for path", "error", err)
		return true, nil
	} else {
		slog.Info("Exists:: Path checked passed without any errors")
	}
	if errors.Is(err, fs.ErrNotExist) {
		slog.Error("Exists:: Path does not exist", "path", path)
		return false, nil
	}
	slog.Info("Exists:: File check path completed without any errors")
	return false, err
}

func PickDomain() string { return constants.TrackerDomains[rand.Intn(len(constants.TrackerDomains))] }

func PrintProgress(label string, done, total int) {
	if total == 0 {
		return
	}
	const width = 30
	filled := done * width / total
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	fmt.Printf("\r    %-25s [%s] %d/%d", label, bar, done, total)
}

func loremSentence(words int) string {
	parts := make([]string, words)
	for i := range parts {
		w := constants.LoremWords[rand.Intn(len(constants.LoremWords))]
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

func LoremDocument() string {
	paras := rand.Intn(6) + 3
	out := make([]string, paras)
	for i := range out {
		out[i] = loremParagraph()
	}
	return strings.Join(out, "\n\n")
}

func RandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func RandomTimestamp() string {
	t := time.Now().Add(-time.Duration(rand.Intn(30*24)) * time.Hour)
	return t.Format("2006-01-02T15:04:05Z")
}

// desktopPath returns the current user's desktop directory across platforms.
func DesktopPath() (string, error) {
	home, err := os.UserHomeDir()
	slog.Info("DesktopPath:: trying to get the user's desktop dir")
	if err != nil {
		slog.Error("DesktopPath:: There was an error while getting the user's homedir", "error", err)
		return "", err
	} else {
		slog.Info("DesktopPath:: There was no error while getting the user's homedir")
	}
	switch runtime.GOOS {
	case "windows":
		// Respect a custom desktop location set via the registry / shell folder env.
		if p := os.Getenv("USERPROFILE"); p != "" {
			slog.Info("DesktopPath::Windows:Custom desktop location ", "customdesktoplocation", filepath.Join(p, "Desktop"))
			return filepath.Join(p, "Desktop"), nil
		}
		slog.Info("DesktopPath::windows:Desktop location", "path", filepath.Join(home, "Desktop"))
		return filepath.Join(home, "Desktop"), nil
	default:
		// macOS and Linux both use ~/Desktop by convention.
		slog.Info("DesktopPath::Mac_LINUX:Desktop location", "path", filepath.Join(home, "Desktop"))
		return filepath.Join(home, "Desktop"), nil
	}
}

func GetFakerDirectoryOnDesktop() (string, error) {
	desktopDir, err := DesktopPath()
	slog.Info("GetFakerDirectoryOnDesktop:: Desktop Directory", "desktopdir", desktopDir)

	if err != nil {
		slog.Error("GetFakerDirectoryOnDesktop:: There was an error while getting the desktop directory", "error", err)
		return "", fmt.Errorf("resolve desktop: %w", err)
	}

	dir := filepath.Join(desktopDir, constants.FakerDir)
	slog.Info("GetFakerDirectoryOnDesktop:: Complete Path", "completePath", dir)
	dirExists, err := Exists(dir)

	if err != nil {
		slog.Error("GetFakerDirectoryOnDesktop:: There was an error while checking if the dir exists or not", "error", err, "dir", dir)
		fmt.Println("It seems that there was an error while checking if the directory exists or not. Bailing out. ")
		os.Exit(1)
	}

	switch dirExists {
	case true:
		fmt.Println("Directory already exists. Will create a new dir with similar name")
		slog.Info("GetFakerDirectoryOnDesktop:: The faker directory exists. will create a new faker directory", "dir", dir)
		dir = filepath.Join(desktopDir, fmt.Sprintf("%s_%s", constants.FakerDir, RandomString(5)))
		slog.Info("GetFakerDirectoryOnDesktop:: The faker directory exists. will create a new faker directory, New Directory Path", "newDir", dir)
		
	case false:
		fmt.Println("Directory does not exist. Go ahead , create a new one")
		slog.Info("GetFakerDirectoryOnDesktop:: Directory does not exist. Will create a new one!")
	}

	// everyting was good, now go ahead and create the dir
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Error("GetFakerDirectoryOnDesktop:: There was an error while creating a new faker directory.", "error", err, "path", dir)
		return "", err
	}
	return dir, nil
}

// chromeDefaultPaths returns (executablePath, profileDir) for the current OS.
func ChromeDefaultPaths() (exePath, profileDir string) {
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

// waitForChromeToClose blocks until Chrome is no longer running,
// prompting the user to close it first.
func WaitForChromeToClose() {
	if !isChromeRunning() {
		slog.Info("Chrome is not running, we are good to go, will proceed ahead!")
		return
	} else {
		slog.Info("Chrome is currently Running. Will ask the user to close it")
	}
	fmt.Println()
	fmt.Println("WARNING: Google Chrome is currently running.")
	fmt.Println("Please close Chrome completely, then press Enter to continue (or Ctrl+C to abort).")
	scanner := bufio.NewScanner(os.Stdin)

	if err := scanner.Err(); err != nil {
		slog.Error("We tried closing the browser but we failed.", "Error", err)
		log.Fatal("Please close the Chrome browser. We tried and we failed. Apologies. PLease run this script after! If the issues persists, please try restarting your machine")
	}

	for {
		scanner.Scan()
		if !isChromeRunning() {
			fmt.Println("Chrome is now closed. Proceeding...")
			slog.Info("Chrome is now closed. Proceeding...")
			return
		}
		fmt.Println("Chrome is still running. Please close it and press Enter again.")
		slog.Info("Chrome is still running. PLease close it!")
	}
}

// toWebKitTime converts a Go time to Chrome's WebKit timestamp format
// (microseconds elapsed since Jan 1, 1601).
func ToWebKitTime(t time.Time) int64 {
	const epochDelta = 11644473600 * 1_000_000 // microseconds
	return t.UnixMicro() + epochDelta
}

// simple , yet holistic task runner, lol
func Run(label string, enabled bool, count int, fn func(string, int) (int, error), baseDir string) int {
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

func RunWithoutBaseDir(label string, enabled bool, count int, fn func(int) (int, error)) int {
	if !enabled {
		fmt.Printf("  %-40s skipped (disabled in config)\n", label)
		return 0
	}
	fmt.Printf("  %-40s ", label)
	n, err := fn(count)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return 0
	}
	fmt.Printf("done (%d)\n", n)
	return n
}

func DownloadEicar() (string, error) {
	destDir := os.TempDir()
	slog.Info("DownloadEicar:: Download Directory", "directory", destDir)
	zipPath := filepath.Join(destDir, "eicar_com.zip")
	if err := downloadFile(constants.EICAR_Url, zipPath, "Downloading EICAR"); err != nil {
		slog.Error("DownloadEicar:: There was an error while downloading file. Error follows", "error", err, "zippath", zipPath)
		return "", err
	}
	slog.Info("DownloadEicar:: Zip path", "zippath", zipPath)
	return zipPath, nil
}

func ExtractAndRunEicar(zipPath, destDir string, virusModel models.VirusConfig) {
	checkedPath, err := extractZip(zipPath, destDir, "Extracting EICAR")
	parentDir := filepath.Dir(filepath.Clean(checkedPath))
	fmt.Println(parentDir)
	// we create the iso on the desktiop of the current user
	isoDirPath, err := DesktopPath()
	if err != nil {
		log.Fatalf("An error happened while getting desktop path , error → %s\n", err)
	}

	// check if we need to create a mount drive
	// we need to do it before execute, otherwise the file may be quarantined when executed by the AV
	if virusModel.CreateISO == true {
		parentDirExists, err := Exists(parentDir)
		if err != nil {
			log.Fatalf("Could not get the user's desktop dir. Error → %s\n", err)
		}
		if parentDirExists == false {
			log.Fatal("we were unable to get user's desktop location.")
		}

		isoName := fmt.Sprintf("faker_%s.iso", RandomString(5))
		desktopISOLocation := filepath.Join(isoDirPath, isoName)

		if err := createISO(parentDir, desktopISOLocation); err != nil {
			log.Fatalf("An error happened while creating ISO, error follows →%s", err)
		}
		// now check if we need to mount it too
		if virusModel.MountISO == true {
			mountPoint, err := mountISO(desktopISOLocation)

			if err != nil {
				log.Fatalf("Could not mount the ISO, error is → %s\n", err)
			}
			fmt.Printf("Mounted the ISO on %s\n", mountPoint)
		} else {
			fmt.Println("Will not mount the generated ISO!")
		}
	}

	if virusModel.AutoExecute == true {
		// all these things need to happen if execute is set to true
		// execute from the mounted ISO when available so AV real-time scanning
		// of the staging directory does not block the run
		virusFileName := filepath.Join(destDir, constants.EICAR_FILE_NAME)
		exists, err := Exists(virusFileName)

		if err != nil {
			log.Fatalf("There was an error while checking for the existing of the file. The error is → %s", virusFileName)
		}

		if exists == false {
			log.Fatalf("File does not exist - %s", virusFileName)
		}

		fmt.Printf("Will execute the virus file now, Virus file is here → %s\n", virusFileName)

		result, err := executeAFile(virusFileName, "")
		if err != nil {
			log.Fatalf("There was an error while executing the virus file , error is → %s", err)
		}
		fmt.Printf("executioon result → %s", result)
	}
}

// local helpers
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

func downloadFile(url, destPath, label string) error {
	slog.Info("DownloadFile:: download a file", "url", url, "destpath", destPath, "label", label)
	resp, err := http.Get(url)
	if err != nil {
		slog.Error("DownloadFile:: Error was encountered while requesting url", "error", err, "url", url)
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(destPath)
	if err != nil {
		slog.Error("DownloadFile:: Error was encountered while creating file", "error", err, "destpath", destPath)
		return err
	}
	defer out.Close()

	total := int(resp.ContentLength)
	buf := make([]byte, 4096)
	done := 0
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
			done += n
			PrintProgress(label, done, total)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Error("DownloadFile:: Error was encountered while downloading", "error", err)
			return err
		}
	}
	slog.Info("DownloadFile:: No errors were encountered and the file was downloaded successfully", "destdir", destPath)
	fmt.Println()
	return nil
}

func extractZip(zipPath, destDir, label string) (string, error) {
	slog.Info("ExtractZip:: Inpur parameters", "zipPath", zipPath, "destDir", destDir, "label", label)
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		slog.Error("ExtractZIP:: Error was encountered while extracting zip (zip.OpenReader)", "error", err)
		return "", err
	}
	defer r.Close()

	var lastPath string
	total := len(r.File)
	for i, f := range r.File {
		PrintProgress(label, i+1, total)

		dstPath := filepath.Join(destDir, f.Name)
		dst, err := os.Create(dstPath)
		if err != nil {
			slog.Error("ExtractZIP:: Error was encountered while extracting zip (os.create)", "error", err)
			return "", err
		}
		src, err := f.Open()
		if err != nil {
			slog.Error("ExtractZIP:: Error was encountered while extracting zip (f.open)", "error", err)
			dst.Close()
			return "", err
		}
		io.Copy(dst, src)
		src.Close()
		dst.Close()
		lastPath = dstPath
	}
	fmt.Println()
	slog.Info("ExtractZip:: Just before returning", "lastpath", lastPath)
	return lastPath, nil
}

func executeAFile(path string, args ...string) (string, error) {
	slog.Info("ExecuteAFile::going to execute a file", "file", path, "args", args)
	cmd := exec.Command(path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("executeAFile:: There was an error while executing a file", "error", err, "path", path, "args", args)
		return string(output), fmt.Errorf("execution failed: %w", err)
	} else {
		slog.Info("Command was executed successfully", "output", string(output))
	}
	return string(output), nil
}
