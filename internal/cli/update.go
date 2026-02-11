package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	githubRepo    = "ishaan812/devlog"
	githubAPIBase = "https://api.github.com/repos/" + githubRepo
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update devlog to the latest version",
	Long: `Update devlog to the latest release from GitHub.

Downloads the latest binary for your platform and replaces the current one.`,
	RunE: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

// githubRelease is the subset of the GitHub release API we need.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func runUpdate(cmd *cobra.Command, args []string) error {
	infoColor := color.New(color.FgCyan)
	successColor := color.New(color.FgGreen)
	errorColor := color.New(color.FgRed)
	dimColor := color.New(color.FgHiBlack)

	// 1. Find the currently running binary
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}

	infoColor.Println("Checking for updates...")
	dimColor.Printf("Current binary: %s\n", execPath)

	// 2. Fetch latest release from GitHub
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Fetching latest release..."
	s.Start()

	release, err := fetchLatestRelease()
	s.Stop()
	if err != nil {
		errorColor.Printf("Failed to check for updates: %v\n", err)
		return err
	}

	infoColor.Printf("Latest version: %s\n", release.TagName)

	// 3. Find the right asset for this platform
	assetName := binaryAssetName()
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		errorColor.Printf("No binary found for your platform (%s/%s)\n", runtime.GOOS, runtime.GOARCH)
		dimColor.Println("Available assets:")
		for _, a := range release.Assets {
			dimColor.Printf("  - %s\n", a.Name)
		}
		return fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// 4. Download the new binary
	fmt.Println()
	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Downloading %s...", assetName)
	s.Start()

	tmpPath := execPath + ".update"
	if err := downloadFile(downloadURL, tmpPath); err != nil {
		s.Stop()
		os.Remove(tmpPath)
		errorColor.Printf("Download failed: %v\n", err)
		return err
	}
	s.Stop()

	// 5. Make it executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod: %w", err)
	}

	// 6. Replace the old binary
	//    Rename is atomic on the same filesystem.
	backupPath := execPath + ".bak"
	os.Remove(backupPath) // clean up any previous backup

	if err := os.Rename(execPath, backupPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to back up current binary: %w", err)
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		// Try to restore the backup
		_ = os.Rename(backupPath, execPath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	os.Remove(backupPath)

	fmt.Println()
	successColor.Printf("Updated devlog to %s!\n", release.TagName)
	return nil
}

func fetchLatestRelease() (*githubRelease, error) {
	req, err := http.NewRequest("GET", githubAPIBase+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "devlog-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parse release: %w", err)
	}
	return &release, nil
}

func binaryAssetName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go arch names to the build names used in releases
	arch := goarch
	if arch == "amd64" {
		arch = "amd64"
	}

	name := fmt.Sprintf("devlog-%s-%s", goos, arch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url) // #nosec G107 - URL is from trusted GitHub API
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}
