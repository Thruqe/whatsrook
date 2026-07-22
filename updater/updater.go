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
	"strconv"
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

type Version struct {
	Major int
	Minor int
	Patch int
	Raw   string
}

type UpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	HasNewVersion  bool
	Updated        bool
	IsBeta         bool
	Method         string // "git" or "release"
	Message        string
}

// ParseVersion converts a semantic version string (e.g. "4.0.0" or "v4.0.0") to structured Version with integer components.
func ParseVersion(raw string) (Version, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "v")

	parts := strings.Split(clean, ".")
	if len(parts) < 3 {
		return Version{Raw: raw}, fmt.Errorf("invalid semver format: %s", raw)
	}

	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patchStr, _, _ := strings.Cut(parts[2], "-")
	patch, err3 := strconv.Atoi(patchStr)

	if err1 != nil || err2 != nil || err3 != nil {
		return Version{Raw: raw}, fmt.Errorf("non-numeric semver component in %s", raw)
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Raw:   raw,
	}, nil
}

// Compare returns 1 if v > other, -1 if v < other, 0 if v == other.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major > other.Major {
			return 1
		}
		return -1
	}
	if v.Minor != other.Minor {
		if v.Minor > other.Minor {
			return 1
		}
		return -1
	}
	if v.Patch != other.Patch {
		if v.Patch > other.Patch {
			return 1
		}
		return -1
	}
	return 0
}

// ReadLocalVersion parses local version.toml.
func ReadLocalVersion(versionPath string) (string, error) {
	data, err := os.ReadFile(versionPath)
	if err != nil {
		return "", err
	}
	return parseVersionFromTOML(string(data))
}

func parseVersionFromTOML(content string) (string, error) {
	lines := strings.SplitSeq(content, "\n")
	for line := range lines {
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

// FetchRemoteVersion fetches raw version.toml from GitHub master branch.
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
	return parseVersionFromTOML(string(body))
}

// IsGitRepo checks if current workspace or executable environment is a Git repository.
func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	if err := cmd.Run(); err == nil {
		return true
	}

	// Check if running via go run
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.") || strings.Contains(arg, "go-build") {
			return true
		}
	}

	if _, err := os.Stat(".git"); err == nil {
		return true
	}
	return false
}

// CheckUpdate compares local version.toml with remote version.toml using numerical component parsing.
func CheckUpdate() (*UpdateResult, error) {
	localStr, err := ReadLocalVersion(VersionFile)
	if err != nil {
		exePath, errExe := os.Executable()
		if errExe == nil {
			localStr, err = ReadLocalVersion(filepath.Join(filepath.Dir(exePath), VersionFile))
		}
		if err != nil {
			localStr = "0.0.0"
		}
	}

	remoteStr, err := FetchRemoteVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote version: %w", err)
	}

	localVer, errLocal := ParseVersion(localStr)
	remoteVer, errRemote := ParseVersion(remoteStr)

	res := &UpdateResult{
		CurrentVersion: localStr,
		LatestVersion:  remoteStr,
	}

	if IsGitRepo() {
		res.Method = "git"
	} else {
		res.Method = "release"
	}

	if errLocal == nil && errRemote == nil {
		res.HasNewVersion = remoteVer.Compare(localVer) > 0
	} else {
		res.HasNewVersion = localStr != remoteStr
	}

	return res, nil
}

// PerformUpdate performs either stable release/git update or beta/nightly update.
func PerformUpdate(isBeta bool) (*UpdateResult, error) {
	check, err := CheckUpdate()
	if err != nil && !isBeta {
		return nil, err
	}
	if check == nil {
		check = &UpdateResult{IsBeta: isBeta}
	} else {
		check.IsBeta = isBeta
	}

	if IsGitRepo() {
		check.Method = "git"
		outStash, errStash := exec.Command("git", "stash").CombinedOutput()
		if errStash != nil {
			return nil, fmt.Errorf("git stash failed: %s (%w)", strings.TrimSpace(string(outStash)), errStash)
		}

		outPull, errPull := exec.Command("git", "pull").CombinedOutput()
		if errPull != nil {
			return nil, fmt.Errorf("git pull failed: %s (%w)", strings.TrimSpace(string(outPull)), errPull)
		}

		outBuild, errBuild := exec.Command("go", "build", "-o", "whatsrook", ".").CombinedOutput()
		if errBuild != nil {
			return nil, fmt.Errorf("rebuild failed after git pull: %s (%w)", strings.TrimSpace(string(outBuild)), errBuild)
		}

		check.Updated = true
		check.Message = fmt.Sprintf("Successfully updated via Git (git stash & git pull). Local version: %s, Remote version: %s.", check.CurrentVersion, check.LatestVersion)
		return check, nil
	}

	// Release update
	check.Method = "release"
	tag := "latest"
	if isBeta {
		tag = "alpha"
	}

	err = downloadReleaseAsset(tag)
	if err != nil {
		return nil, fmt.Errorf("failed to download release (%s): %w", tag, err)
	}

	check.Updated = true
	check.Message = fmt.Sprintf("Successfully updated to %s build (%s -> %s).", tag, check.CurrentVersion, check.LatestVersion)
	return check, nil
}

func downloadReleaseAsset(tag string) error {
	assetName := fmt.Sprintf("whatsrook-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	if tag == "latest" {
		downloadURL = fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s", RepoOwner, RepoName, assetName)
	} else {
		downloadURL = fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", RepoOwner, RepoName, tag, assetName)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading asset from %s", resp.StatusCode, downloadURL)
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
			return fmt.Errorf("binary not found in release archive")
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
