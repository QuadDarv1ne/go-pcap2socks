// Package updater provides automatic update functionality for go-pcap2socks.
// Checks GitHub releases and downloads updates for the current platform.
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
)

// Pre-defined errors for updater operations
var (
	ErrUpdateCheckFailed    = errors.New("failed to check for updates")
	ErrUpdateDownloadFailed = errors.New("failed to download update")
	ErrUpdateApplyFailed    = errors.New("failed to apply update")
	ErrInvalidVersion       = errors.New("invalid version format")
	ErrNoAssetForPlatform   = errors.New("no asset found for current platform")
)

// Updater constants
const (
	// GitHub API endpoint for releases
	GitHubReleasesAPI = "https://api.github.com/repos/DaniilSokolyuk/go-pcap2socks/releases/latest"
	// Default check interval
	DefaultCheckInterval = 24 * time.Hour
)

// Release represents a GitHub release
type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	HTMLURL     string  `json:"html_url"`
	Assets      []Asset `json:"assets"`
}

// Asset represents a release asset
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Updater handles automatic updates
type Updater struct {
	currentVersion string
	checkInterval  time.Duration
	httpClient     *http.Client
	onUpdate       func(oldVersion, newVersion string)
	checkMu        sync.Mutex      // Protects checkStop and checkRunning
	checkStop      chan struct{}
	checkRunning   bool
}

// NewUpdater creates a new Updater instance
func NewUpdater(currentVersion string) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		checkInterval:  DefaultCheckInterval,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				ForceAttemptHTTP2: true,
			},
		},
	}
}

// SetCheckInterval sets the interval for checking updates
func (u *Updater) SetCheckInterval(interval time.Duration) {
	u.checkInterval = interval
}

// SetOnUpdate sets the callback function to call when an update is available
func (u *Updater) SetOnUpdate(callback func(oldVersion, newVersion string)) {
	u.onUpdate = callback
}

// CheckForUpdates checks if a new version is available.
// Returns the release info, whether it's newer, and any error.
func (u *Updater) CheckForUpdates() (*Release, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GitHubReleasesAPI, nil)
	if err != nil {
		return nil, false, fmt.Errorf("%w: failed to create request: %v", ErrUpdateCheckFailed, err)
	}

	// Set required headers for GitHub API
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "go-pcap2socks-updater")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("%w: failed to fetch releases: %v", ErrUpdateCheckFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("%w: GitHub API returned status %d", ErrUpdateCheckFailed, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("%w: failed to read response body: %v", ErrUpdateCheckFailed, err)
	}

	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, false, fmt.Errorf("%w: failed to parse release: %v", ErrUpdateCheckFailed, err)
	}

	// Check if this is a newer version
	isNewer, err := u.isNewerVersion(release.TagName)
	if err != nil {
		return nil, false, fmt.Errorf("%w: failed to compare versions: %v", ErrInvalidVersion, err)
	}

	return &release, isNewer, nil
}

// DownloadUpdate downloads the latest release for the current platform
func (u *Updater) DownloadUpdate(release *Release) (string, error) {
	// Find the appropriate asset for current platform
	assetName := u.getAssetName()
	var asset *Asset
	for _, a := range release.Assets {
		if a.Name == assetName {
			asset = &a
			break
		}
	}

	if asset == nil {
		return "", fmt.Errorf("no asset found for platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Download the asset
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Get executable path
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Create temporary file for download
	tempFile := execPath + ".new"
	out, err := os.Create(tempFile)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer out.Close()

	// Copy downloaded file
	if _, err := io.Copy(out, resp.Body); err != nil {
		os.Remove(tempFile)
		return "", fmt.Errorf("failed to write update: %w", err)
	}

	// Set executable permissions
	if err := os.Chmod(tempFile, 0755); err != nil {
		os.Remove(tempFile)
		return "", fmt.Errorf("failed to set permissions: %w", err)
	}

	return tempFile, nil
}

// ApplyUpdate replaces the current executable with the new version
func (u *Updater) ApplyUpdate(newVersion string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	tempFile := execPath + ".new"
	backupFile := execPath + ".old"

	// Check if temp file exists
	if _, err := os.Stat(tempFile); os.IsNotExist(err) {
		return fmt.Errorf("update file not found")
	}

	// Create backup
	if err := copyFile(execPath, backupFile); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Replace executable
	if err := os.Rename(tempFile, execPath); err != nil {
		// Restore backup
		os.Rename(backupFile, execPath)
		return fmt.Errorf("failed to replace executable: %w", err)
	}

	slog.Info("Update applied successfully", "new_version", newVersion)

	// Remove backup after successful update (in background with error handling)
	goroutine.SafeGo(func() {
		time.Sleep(5 * time.Minute)
		if err := os.Remove(backupFile); err != nil {
			slog.Debug("Failed to remove backup file", "file", backupFile, "err", err)
		}
	})

	return nil
}

// StartAutoCheck starts periodic update checking in background
func (u *Updater) StartAutoCheck() {
	u.checkMu.Lock()
	if u.checkRunning {
		u.checkMu.Unlock()
		return
	}

	// Stop any existing check first
	if u.checkStop != nil {
		close(u.checkStop)
	}

	u.checkStop = make(chan struct{})
	u.checkRunning = true
	u.checkMu.Unlock()

	goroutine.SafeGo(func() {
		ticker := time.NewTicker(u.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				release, isNewer, err := u.CheckForUpdates()
				if err != nil {
					slog.Debug("Update check failed", "err", err)
					continue
				}

				if isNewer {
					slog.Info("New version available",
						"current", u.currentVersion,
						"latest", release.TagName,
						"url", release.HTMLURL)

					if u.onUpdate != nil {
						u.onUpdate(u.currentVersion, release.TagName)
					}
				}
			case <-u.checkStop:
				slog.Info("Update autocheck stopped")
				return
			}
		}
	})
}

// StopAutoCheck stops periodic update checking
func (u *Updater) StopAutoCheck() {
	u.checkMu.Lock()
	defer u.checkMu.Unlock()
	if !u.checkRunning {
		return
	}
	close(u.checkStop)
	u.checkRunning = false
}

// isNewerVersion compares versions and returns true if latest is newer
func (u *Updater) isNewerVersion(latest string) (bool, error) {
	// Remove 'v' prefix if present
	current := u.currentVersion
	if len(current) > 0 && current[0] == 'v' {
		current = current[1:]
	}
	if len(latest) > 0 && latest[0] == 'v' {
		latest = latest[1:]
	}

	// Simple version comparison (assumes semver: major.minor.patch)
	return compareVersions(current, latest) > 0, nil
}

// compareVersions compares two version strings
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	parts1 := parseVersion(v1)
	parts2 := parseVersion(v2)

	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		if parts1[i] > parts2[i] {
			return 1
		}
		if parts1[i] < parts2[i] {
			return -1
		}
	}

	if len(parts1) > len(parts2) {
		return 1
	}
	if len(parts1) < len(parts2) {
		return -1
	}

	return 0
}

// parseVersion parses a version string into integer parts
func parseVersion(v string) []int {
	parts := make([]int, 0, 3) // Pre-allocate for typical semver
	for _, part := range splitString(v, ".") {
		if n := parseInt(part); n >= 0 {
			parts = append(parts, n)
		}
	}
	return parts
}

// splitString splits a string by separator
func splitString(s, sep string) []string {
	// Count separators for pre-allocation
	count := 1
	for i := 0; i < len(s); i++ {
		if s[i:i+len(sep)] == sep {
			count++
			i += len(sep) - 1
		}
	}

	parts := make([]string, 0, count)
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			parts = append(parts, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// parseInt parses an integer from string
//
//go:inline
func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// getAssetName returns the asset name for current platform
func (u *Updater) getAssetName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map GOARCH to common naming
	switch goarch {
	case "amd64":
		goarch = "x86_64"
	case "arm64":
		goarch = "arm64"
	case "386":
		goarch = "i386"
	}

	// Map GOOS to common naming
	switch goos {
	case "windows":
		return fmt.Sprintf("go-pcap2socks-%s-%s.exe", goos, goarch)
	case "darwin":
		return fmt.Sprintf("go-pcap2socks-%s-%s", goos, goarch)
	default:
		return fmt.Sprintf("go-pcap2socks-%s-%s", goos, goarch)
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}

// Restart restarts the application with the same arguments
func Restart() error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	// Prepare command
	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start new instance
	if err := cmd.Start(); err != nil {
		return err
	}

	// Exit current instance
	os.Exit(0)
	return nil
}

// GetExecutableDir returns the directory containing the executable
//
//go:inline
func GetExecutableDir() string {
	execPath, err := os.Executable()
	if err != nil {
		return "."
	}
	return path.Dir(execPath)
}
