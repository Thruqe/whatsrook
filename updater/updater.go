// Self-update mechanism: downloads and applies pre-built releases matching host system from GitHub.
package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
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

	"whatsrook/store/sqlstore"
)

const (
	RepoOwner     = "Thruqe"
	RepoName      = "whatsrook"
	VersionFile   = "version.toml"
	VersionGithub = "https://raw.githubusercontent.com/Thruqe/whatsrook/refs/heads/master/version.toml"
	ChannelKey    = "update_channel" // "stable" or "beta"
)

// Version holds a semantic version (major.minor.patch).
type Version struct {
	Major int
	Minor int
	Patch int
	Raw   string
}

// UpdateResult describes the outcome of an update check or update operation.
type UpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	HasNewVersion  bool
	Updated        bool
	IsBeta         bool
	Platform       string
	Message        string
}

// GetPlatform returns operating system and architecture string (e.g. linux/amd64).
func GetPlatform() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

// GetChannel gets configured update channel ("stable" or "beta").
func GetChannel(ctx context.Context, store *sqlstore.SQLStore) string {
	if store == nil {
		return "stable"
	}
	ch, err := store.GetSetting(ctx, ChannelKey)
	if err != nil || ch == "" {
		return "stable"
	}
	return strings.ToLower(ch)
}

// SetChannel sets update channel ("stable" or "beta").
func SetChannel(ctx context.Context, store *sqlstore.SQLStore, channel string) error {
	if store == nil {
		return fmt.Errorf("settings store unavailable")
	}
	channel = strings.ToLower(channel)
	if channel != "stable" && channel != "beta" {
		return fmt.Errorf("invalid channel %q", channel)
	}
	return store.PutSetting(ctx, ChannelKey, channel)
}

// ParseVersion converts semver string to Version struct.
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

// Compare compares two versions, returning -1/0/+1 like cmp.Compare.
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

// ReadLocalVersion reads and parses the version string from a local version.toml file.
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

// FetchRemoteVersion fetches the latest version string from remote repository.
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

// CheckUpdate compares local and remote versions for current platform.
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
		Platform:       GetPlatform(),
	}

	if errLocal == nil && errRemote == nil {
		res.HasNewVersion = remoteVer.Compare(localVer) > 0
	} else {
		res.HasNewVersion = localStr != remoteStr
	}

	return res, nil
}

// PerformUpdate downloads the system-matching pre-built binary release and replaces the binary.
func PerformUpdate(isBeta bool) (*UpdateResult, error) {
	check, err := CheckUpdate()
	if err != nil && !isBeta {
		return nil, err
	}
	if check == nil {
		check = &UpdateResult{
			IsBeta:   isBeta,
			Platform: GetPlatform(),
		}
	} else {
		check.IsBeta = isBeta
	}

	tag := "latest"
	if isBeta {
		tag = "alpha"
	}

	if err := downloadAndApplyRelease(tag); err != nil {
		return nil, fmt.Errorf("failed to update binary for %s: %w", GetPlatform(), err)
	}

	check.Updated = true
	check.Message = fmt.Sprintf("Successfully updated binary for platform %s to %s release (%s -> %s).", GetPlatform(), tag, check.CurrentVersion, check.LatestVersion)
	return check, nil
}

func downloadAndApplyRelease(tag string) error {
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
		return fmt.Errorf("HTTP %d downloading %s from %s", resp.StatusCode, assetName, downloadURL)
	}

	exePath, err := os.Executable()
	if err != nil {
		exePath = os.Args[0]
	}
	exeDir := filepath.Dir(exePath)

	tmpBinary := exePath + ".tmp"
	out, err := os.OpenFile(tmpBinary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	gzr, errGz := gzip.NewReader(resp.Body)
	if errGz != nil {
		out.Close()
		_ = os.Remove(tmpBinary)
		return fmt.Errorf("failed to decompress archive: %w", errGz)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	foundBinary := false

	for {
		hdr, errHdr := tr.Next()
		if errHdr == io.EOF {
			break
		}
		if errHdr != nil {
			out.Close()
			_ = os.Remove(tmpBinary)
			return errHdr
		}

		baseName := filepath.Base(hdr.Name)
		if baseName == "whatsrook" || baseName == "whatsrook.exe" {
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				_ = os.Remove(tmpBinary)
				return err
			}
			foundBinary = true
		} else if baseName == VersionFile {
			versionDest := filepath.Join(exeDir, VersionFile)
			vFile, errV := os.OpenFile(versionDest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if errV == nil {
				_, _ = io.Copy(vFile, tr)
				vFile.Close()
			}
		}
	}

	out.Close()

	if !foundBinary {
		_ = os.Remove(tmpBinary)
		return fmt.Errorf("binary not found in release archive %s", assetName)
	}

	if runtime.GOOS == "windows" {
		oldPath := exePath + ".old"
		_ = os.Remove(oldPath)
		_ = os.Rename(exePath, oldPath)
	}

	if err := os.Rename(tmpBinary, exePath); err != nil {
		return fmt.Errorf("failed to replace executable: %w", err)
	}

	return nil
}

// RestartProcess replaces current process with the updated binary.
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
