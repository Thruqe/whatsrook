// Package updater provides self-update capabilities for WhatsRook matching system package manager designs (brew/apt/dnf style).
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

	"whatsrook/wa-core/store/sqlstore"
)

const (
	DefaultRepoOwner   = "Thruqe"
	DefaultRepoName    = "whatsrook"
	DefaultVersionFile = "version.toml"
	DefaultVersionURL  = "https://raw.githubusercontent.com/Thruqe/whatsrook/refs/heads/master/version.toml"
	ChannelKey         = "update_channel" // "stable" or "beta"
)

// Backward-compatible exports for external callers.
const (
	RepoOwner     = DefaultRepoOwner
	RepoName      = DefaultRepoName
	VersionFile   = DefaultVersionFile
	VersionGithub = DefaultVersionURL
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

// Options configures an Updater instance.
type Options struct {
	RepoOwner   string
	RepoName    string
	VersionFile string
	Channel     string    // "stable" or "beta"
	Out         io.Writer // Writer for progress logs (e.g. os.Stdout)
	HTTPClient  *http.Client
}

// Updater manages checking for and applying application upgrades.
type Updater struct {
	opts Options
}

// New returns a new Updater initialized with the provided Options.
func New(opts Options) *Updater {
	if opts.RepoOwner == "" {
		opts.RepoOwner = DefaultRepoOwner
	}
	if opts.RepoName == "" {
		opts.RepoName = DefaultRepoName
	}
	if opts.VersionFile == "" {
		opts.VersionFile = DefaultVersionFile
	}
	if opts.Channel == "" {
		opts.Channel = "stable"
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Updater{opts: opts}
}

// DefaultUpdater returns an Updater with default settings.
func DefaultUpdater() *Updater {
	return New(Options{})
}

// SetOutput sets the destination writer for progress logging.
func (u *Updater) SetOutput(w io.Writer) {
	u.opts.Out = w
}

func (u *Updater) logf(format string, args ...any) {
	if u.opts.Out != nil {
		fmt.Fprintf(u.opts.Out, format+"\n", args...)
	}
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

// ParseVersion converts a semver string into a Version struct.
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

// GetAppVersion attempts to read version from local version.toml,
// and if unavailable or failing, fetches it from GitHub, with fallback to "5.1.0".
func GetAppVersion() string {
	if ver, err := ReadLocalVersion(DefaultVersionFile); err == nil && strings.TrimSpace(ver) != "" {
		return strings.TrimSpace(ver)
	}
	if remoteVer, err := FetchRemoteVersion(); err == nil && strings.TrimSpace(remoteVer) != "" {
		return strings.TrimSpace(remoteVer)
	}
	return "5.1.0"
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

// FetchRemoteVersion fetches the latest version string from the remote repository.
func FetchRemoteVersion() (string, error) {
	return DefaultUpdater().FetchRemoteVersion(context.Background())
}

// FetchRemoteVersion fetches the latest version string using the Updater's configured HTTP client and context.
func (u *Updater) FetchRemoteVersion(ctx context.Context) (string, error) {
	versionURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/refs/heads/master/%s", u.opts.RepoOwner, u.opts.RepoName, u.opts.VersionFile)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := u.opts.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching remote %s", resp.StatusCode, u.opts.VersionFile)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return parseVersionFromTOML(string(body))
}

// CheckUpdate compares local and remote versions for current platform using DefaultUpdater.
func CheckUpdate() (*UpdateResult, error) {
	return DefaultUpdater().Check(context.Background())
}

// Check compares local and remote versions for the configured repository and platform.
func (u *Updater) Check(ctx context.Context) (*UpdateResult, error) {
	u.logf("==> Checking for updates (%s/%s, platform: %s)...", u.opts.RepoOwner, u.opts.RepoName, GetPlatform())

	localStr, err := ReadLocalVersion(u.opts.VersionFile)
	if err != nil {
		exePath, errExe := os.Executable()
		if errExe == nil {
			localStr, err = ReadLocalVersion(filepath.Join(filepath.Dir(exePath), u.opts.VersionFile))
		}
		if err != nil {
			localStr = "0.0.0"
		}
	}

	remoteStr, err := u.FetchRemoteVersion(ctx)
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

	if res.HasNewVersion {
		u.logf("==> Update available! Installed: %s -> Latest: %s", localStr, remoteStr)
	} else {
		u.logf("==> %s is already at the latest version (%s).", u.opts.RepoName, localStr)
	}

	return res, nil
}

// PerformUpdate downloads the system-matching release and replaces the binary using DefaultUpdater.
func PerformUpdate(isBeta bool) (*UpdateResult, error) {
	return DefaultUpdater().Upgrade(context.Background(), isBeta)
}

// Upgrade checks, downloads, and performs an atomic upgrade of the binary release.
func (u *Updater) Upgrade(ctx context.Context, isBeta bool) (*UpdateResult, error) {
	check, err := u.Check(ctx)
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

	u.logf("==> [1/3] Downloading %s release for %s...", tag, GetPlatform())
	if err := u.DownloadAndApply(ctx, tag); err != nil {
		return nil, fmt.Errorf("failed to upgrade binary for %s: %w", GetPlatform(), err)
	}

	check.Updated = true
	check.Message = fmt.Sprintf("Successfully upgraded binary for %s (%s -> %s).", GetPlatform(), check.CurrentVersion, check.LatestVersion)
	u.logf("==> Upgrade complete! %s", check.Message)
	return check, nil
}

// DownloadAndApply downloads a release tar.gz asset, verifies extraction security, and performs atomic binary swap.
func (u *Updater) DownloadAndApply(ctx context.Context, tag string) error {
	assetName := fmt.Sprintf("whatsrook-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	if tag == "latest" {
		downloadURL = fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s", u.opts.RepoOwner, u.opts.RepoName, assetName)
	} else {
		downloadURL = fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", u.opts.RepoOwner, u.opts.RepoName, tag, assetName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := u.opts.HTTPClient.Do(req)
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

	u.logf("==> [2/3] Extracting release payload and verifying paths...")

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
	cleanExeDir := filepath.Clean(exeDir)

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

		// Prevent Zip/Tar Slip (arbitrary file access during extraction)
		relPath := filepath.Clean(hdr.Name)
		if strings.HasPrefix(relPath, "..") || filepath.IsAbs(hdr.Name) || strings.HasPrefix(hdr.Name, "/") || strings.HasPrefix(hdr.Name, "\\") {
			out.Close()
			_ = os.Remove(tmpBinary)
			return fmt.Errorf("illegal archive entry path (Zip Slip attempt): %s", hdr.Name)
		}

		destPath := filepath.Join(exeDir, relPath)
		if !strings.HasPrefix(destPath, cleanExeDir+string(filepath.Separator)) && destPath != cleanExeDir {
			out.Close()
			_ = os.Remove(tmpBinary)
			return fmt.Errorf("archive entry path escapes destination directory: %s", hdr.Name)
		}

		if strings.HasPrefix(relPath, "cli/resources") || strings.HasPrefix(relPath, "resources") || strings.HasPrefix(relPath, "prompts") {
			if hdr.Typeflag == tar.TypeDir {
				_ = os.MkdirAll(destPath, 0755)
			} else {
				_ = os.MkdirAll(filepath.Dir(destPath), 0755)
				resFile, errRes := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
				if errRes == nil {
					_, _ = io.Copy(resFile, tr)
					resFile.Close()
				}
			}
			continue
		}

		baseName := filepath.Base(hdr.Name)
		switch baseName {
		case "whatsrook", "whatsrook.exe":
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				_ = os.Remove(tmpBinary)
				return err
			}
			foundBinary = true
		case u.opts.VersionFile:
			versionDest := filepath.Join(exeDir, u.opts.VersionFile)
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

	u.logf("==> [3/3] Performing atomic binary swap with rollback safety...")

	backupPath := exePath + ".bak"
	_ = os.Remove(backupPath)

	// Backup current working binary
	if err := os.Rename(exePath, backupPath); err != nil {
		_ = os.Remove(tmpBinary)
		return fmt.Errorf("failed to backup existing binary: %w", err)
	}

	// Atomic replace with new binary
	if err := os.Rename(tmpBinary, exePath); err != nil {
		// Rollback to original working binary
		_ = os.Rename(backupPath, exePath)
		_ = os.Remove(tmpBinary)
		return fmt.Errorf("failed to replace executable (rolled back): %w", err)
	}

	// Cleanup backup file
	_ = os.Remove(backupPath)

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
