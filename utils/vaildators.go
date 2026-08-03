// URL matching, JID sanitisation, and message extraction.
package utils

import (
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// IsFacebookURL checks if the URL matches Facebook domain.
func IsFacebookURL(link string) bool {
	return MatchesHost(link, "facebook.com", "fb.com", "fb.watch")
}

// IsInstagramURL checks if the URL matches Instagram domain.
func IsInstagramURL(link string) bool {
	return MatchesHost(link, "instagram.com")
}

// IsTwitterURL checks if the URL matches Twitter/X domain.
func IsTwitterURL(link string) bool {
	return MatchesHost(link, "twitter.com", "x.com")
}

// IsThreadsURL checks if the URL matches Threads domain.
func IsThreadsURL(link string) bool {
	return MatchesHost(link, "threads.net", "threads.com")
}

// IsYouTubeURL checks if the URL matches YouTube domain.
func IsYouTubeURL(link string) bool {
	return MatchesHost(link, "youtube.com", "youtu.be")
}

// IsTikTokURL checks if the URL matches TikTok domain.
func IsTikTokURL(link string) bool {
	return MatchesHost(link, "tiktok.com")
}

// GetPlatformNameFromURL returns the human-readable platform name for a URL.
func GetPlatformNameFromURL(link string) string {
	switch {
	case IsYouTubeURL(link):
		return "YouTube"
	case IsTwitterURL(link):
		return "Twitter"
	case IsInstagramURL(link):
		return "Instagram"
	case IsTikTokURL(link):
		return "TikTok"
	case IsFacebookURL(link):
		return "Facebook"
	case IsThreadsURL(link):
		return "Threads"
	default:
		return "this platform"
	}
}

// MatchesHost parses the URL and checks if its host matches
// any of the given domains (including subdomains like www.).
func MatchesHost(link string, domains ...string) bool {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")

	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// GetGitCommit returns the short commit hash if running inside a Git repository.
func GetGitCommit() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "N/A"
	}
	return strings.TrimSpace(string(out))
}

// SystemMetadata contains runtime system and environment details.
type SystemMetadata struct {
	Version   string
	Commit    string
	OS        string
	Arch      string
	NumCPU    int
	GoVersion string
}

// GetSystemMetadata gathers system metadata for diagnostics and status reporting.
func GetSystemMetadata(version string) SystemMetadata {
	commit := "N/A"
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	if out, err := cmd.Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}

	return SystemMetadata{
		Version:   version,
		Commit:    commit,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
		GoVersion: runtime.Version(),
	}
}
