package helpers

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}


// desktopPath returns the current user's desktop directory across platforms.
func DesktopPath() (string, error) {
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
func ToWebKitTime(t time.Time) int64 {
	const epochDelta = 11644473600 * 1_000_000 // microseconds
	return t.UnixMicro() + epochDelta
}


// local helpers

//
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

