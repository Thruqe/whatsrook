package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var (
	ytdlpMu sync.Mutex
)

const ytDlpDownloadURL = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp"

// GetYTDLPPath returns the path to the yt-dlp binary (preferring ./bin/yt-dlp).
// Automatically downloads the latest standalone binary to ./bin/yt-dlp if not found.
func GetYTDLPPath(ctx context.Context) (string, error) {
	ytdlpMu.Lock()
	defer ytdlpMu.Unlock()

	binDir := "bin"
	if err := os.MkdirAll(binDir, 0755); err != nil {
		mainLog.Warnf("Failed to create bin directory: %v", err)
	}

	execName := "yt-dlp"
	if runtime.GOOS == "windows" {
		execName = "yt-dlp.exe"
	}

	localPath := filepath.Join(binDir, execName)
	if isExecutable(localPath) {
		return localPath, nil
	}

	// Check PATH fallback
	if path, err := exec.LookPath(execName); err == nil {
		return path, nil
	}

	// Download standalone binary directly to ./bin/yt-dlp
	mainLog.Infof("yt-dlp binary not found. Automatically downloading standalone binary to %s...", localPath)
	if err := downloadYTDLP(ctx, localPath); err != nil {
		return "", fmt.Errorf("failed to auto-download yt-dlp: %w", err)
	}

	return localPath, nil
}

// UpdateYTDLP updates the yt-dlp binary in ./bin/yt-dlp using self-update or re-download.
func UpdateYTDLP(ctx context.Context) error {
	ytdlpMu.Lock()
	defer ytdlpMu.Unlock()

	binDir := "bin"
	execName := "yt-dlp"
	if runtime.GOOS == "windows" {
		execName = "yt-dlp.exe"
	}
	targetPath := filepath.Join(binDir, execName)

	if isExecutable(targetPath) {
		mainLog.Infof("Attempting self-update of yt-dlp binary at %s...", targetPath)
		cmd := exec.CommandContext(ctx, targetPath, "-U")
		setSSLBypassEnv(cmd)
		if err := cmd.Run(); err == nil {
			mainLog.Infof("yt-dlp binary successfully updated via -U")
			return nil
		}
	}

	mainLog.Infof("Re-downloading latest yt-dlp release binary to %s...", targetPath)
	return downloadYTDLP(ctx, targetPath)
}

func downloadYTDLP(ctx context.Context, destPath string) error {
	_ = os.MkdirAll(filepath.Dir(destPath), 0755)

	downloadURL := ytDlpDownloadURL
	if runtime.GOOS == "windows" {
		downloadURL = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error %d downloading yt-dlp from %s", resp.StatusCode, downloadURL)
	}

	tmpFile := destPath + ".tmp"
	out, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	_ = out.Close()
	if err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpFile, 0755); err != nil {
			mainLog.Warnf("Failed to set executable permission on %s: %v", tmpFile, err)
		}
	}

	if err := os.Rename(tmpFile, destPath); err != nil {
		data, errRead := os.ReadFile(tmpFile)
		if errRead == nil {
			_ = os.WriteFile(destPath, data, 0755)
		}
		_ = os.Remove(tmpFile)
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(destPath, 0755)
	}

	mainLog.Infof("Successfully downloaded and installed yt-dlp binary to %s", destPath)
	return nil
}

func setSSLBypassEnv(cmd *exec.Cmd) {
	env := os.Environ()
	env = append(env, "PYTHONHTTPSVERIFY=0", "SSL_CERT_FILE=", "PYTHONWARNINGS=ignore")
	cmd.Env = env
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	// On Unix/Linux/Containers, check if executable bit is set OR file is a valid downloaded binary (> 100KB)
	return (info.Mode()&0111 != 0) || (info.Size() > 100000)
}
