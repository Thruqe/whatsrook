package updater_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"whatsrook/cli/updater"
)

func TestParseVersion(t *testing.T) {
	v, err := updater.ParseVersion("4.0.1")
	if err != nil {
		t.Fatalf("unexpected error parsing semver: %v", err)
	}
	if v.Major != 4 || v.Minor != 0 || v.Patch != 1 {
		t.Errorf("unexpected semver components: %+v", v)
	}

	v2, err := updater.ParseVersion("v4.1.0-alpha")
	if err != nil {
		t.Fatalf("unexpected error parsing semver with prefix/suffix: %v", err)
	}
	if v2.Major != 4 || v2.Minor != 1 || v2.Patch != 0 {
		t.Errorf("unexpected semver components: %+v", v2)
	}

	if v2.Compare(v) <= 0 {
		t.Errorf("expected v2 (4.1.0) > v (4.0.1)")
	}
}

func TestReadLocalVersion(t *testing.T) {
	tmpDir := t.TempDir()
	versionPath := filepath.Join(tmpDir, "version.toml")

	content := `version = "4.2.0"`
	if err := os.WriteFile(versionPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test version.toml: %v", err)
	}

	ver, err := updater.ReadLocalVersion(versionPath)
	if err != nil {
		t.Fatalf("unexpected error reading version: %v", err)
	}

	if ver != "4.2.0" {
		t.Errorf("expected 4.2.0, got %s", ver)
	}
}

func TestGetPlatform(t *testing.T) {
	platform := updater.GetPlatform()
	if platform == "" || platform == "/" {
		t.Errorf("expected valid OS/Arch platform string, got %q", platform)
	}
}

func TestUpdaterOptions(t *testing.T) {
	var buf bytes.Buffer
	up := updater.New(updater.Options{
		RepoOwner:   "TestOwner",
		RepoName:    "TestRepo",
		VersionFile: "custom_version.toml",
		Out:         &buf,
	})

	if up == nil {
		t.Fatal("expected non-nil Updater instance")
	}

	up.SetOutput(&buf)
	ctx := context.Background()

	// Perform a check against invalid remote to verify custom options are used
	_, err := up.Check(ctx)
	if err == nil {
		t.Log("check finished without error")
	}

	output := buf.String()
	if output == "" {
		t.Errorf("expected progress output to be written to buffer, got empty")
	}
}
