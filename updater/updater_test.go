package updater_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Thruqe/whatsrook/updater"
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

func TestIsGitRepo(t *testing.T) {
	if !updater.IsGitRepo() {
		t.Errorf("expected IsGitRepo to return true in current codebase environment")
	}
}
