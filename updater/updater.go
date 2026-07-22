package updater

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	RepoOwner     = "Thruqe"
	RepoName      = "whatsrook"
	VersionFile   = "version.toml"
	VersionGithub = "https://raw.githubusercontent.com/Thruqe/whatsrook/refs/heads/master/version.toml"
)

type UpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	Updated        bool
	Method         string // "git" or "release"
	Message        string
}

// ReadLocalVersion parses local version.toml.
func ReadLocalVersion(versionPath string) (string, error) {
	data, err := os.ReadFile(versionPath)
	if err != nil {
		return "", err
	}
	return parseVersionString(string(data))
}

func parseVersionString(content string) (string, error) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, `"'`)
				return val, nil
			}
		}
	}
	return "", fmt.Errorf("version key not found in version.toml")
}

// FetchRemoteVersion fetches raw version.toml from GitHub main branch.
func FetchRemoteVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(VersionGithub)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching remote version.toml", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return parseVersionString(string(body))
}

// IsGitRepo checks if the current working directory or executable directory is in a Git repository.
func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	err := cmd.Run()
	return err == nil
}

// PerformUpdate updates the repository or binary based on whether git is available.
func PerformUpdate() (*UpdateResult, error) {
	localVer, err := ReadLocalVersion(VersionFile)
	if err != nil {
		exePath, errExe := os.Executable()
		if errExe == nil {
			localVer, err = ReadLocalVersion(filepath.Join(filepath.Dir(exePath), VersionFile))
		}
		if err != nil {
			localVer = "unknown"
		}
	}

	remoteVer, err := FetchRemoteVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to check remote version: %w", err)
	}

	res := &UpdateResult{
		CurrentVersion: localVer,
		LatestVersion:  remoteVer,
	}

	if IsGitRepo() {
		res.Method = "git"
		// Git update path: git stash && git pull
		outStash, errStash := exec.Command("git", "stash").CombinedOutput()
		if errStash != nil {
			return nil, fmt.Errorf("git stash failed: %s (%w)", strings.TrimSpace(string(outStash)), errStash)
		}

		outPull, errPull := exec.Command("git", "pull").CombinedOutput()
		if errPull != nil {
			return nil, fmt.Errorf("git pull failed: %s (%w)", strings.TrimSpace(string(outPull)), errPull)
		}

		// Rebuild binary via go build
		outBuild, errBuild := exec.Command("go", "build", "-o", "whatsrook", ".").CombinedOutput()
		if errBuild != nil {
			return nil, fmt.Errorf("rebuild failed after git pull: %s (%w)", strings.TrimSpace(string(outBuild)), errBuild)
		}

		res.Updated = true
		res.Message = fmt.Sprintf("Successfully updated via Git (git stash & git pull). Local version: %s, Remote version: %s.", localVer, remoteVer)
		return res, nil
	}

	// Non-git release download update path
	res.Method = "release"
	if localVer != "unknown" && localVer == remoteVer {
		res.Updated = false
		res.Message = fmt.Sprintf("Already up to date (version %s)", localVer)
		return res, nil
	}

	err = downloadLatestReleaseBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to download release binary: %w", err)
	}

	res.Updated = true
	res.Message = fmt.Sprintf("Successfully updated from version %s to %s via GitHub Release.", localVer, remoteVer)
	return res, nil
}

func downloadLatestReleaseBinary() error {
	assetName := fmt.Sprintf("whatsrook_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	downloadURL := fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s", RepoOwner, RepoName, assetName)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, downloadURL)
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	tmpBinary := exePath + ".new"
	out, err := os.OpenFile(tmpBinary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	gzr, errGz := gzip.NewReader(resp.Body)
	if errGz == nil {
		tr := tar.NewReader(gzr)
		found := false
		for {
			hdr, errHdr := tr.Next()
			if errHdr == io.EOF {
				break
			}
			if errHdr != nil {
				out.Close()
				os.Remove(tmpBinary)
				return errHdr
			}
			if filepath.Base(hdr.Name) == "whatsrook" || filepath.Base(hdr.Name) == "whatsrook.exe" {
				if _, err := io.Copy(out, tr); err != nil {
					out.Close()
					os.Remove(tmpBinary)
					return err
				}
				found = true
				break
			}
		}
		gzr.Close()
		out.Close()
		if !found {
			os.Remove(tmpBinary)
			return fmt.Errorf("binary not found in release tarball")
		}
	} else {
		if _, err := io.Copy(out, resp.Body); err != nil {
			out.Close()
			os.Remove(tmpBinary)
			return err
		}
		out.Close()
	}

	if err := os.Rename(tmpBinary, exePath); err != nil {
		_ = os.Remove(exePath)
		if err := os.Rename(tmpBinary, exePath); err != nil {
			return err
		}
	}

	return nil
}

// RestartProcess restarts the current process using syscall.Exec.
func RestartProcess() error {
	argv := os.Args
	execPath, err := exec.LookPath(argv[0])
	if err != nil {
		execPath, err = os.Executable()
		if err != nil {
			return err
		}
	}

	if runtime.GOOS == "windows" {
		cmd := exec.Command(execPath, argv[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Start(); err != nil {
			return err
		}
		os.Exit(0)
		return nil
	}

	return syscall.Exec(execPath, argv, os.Environ())
}
