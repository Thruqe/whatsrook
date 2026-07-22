package updater_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Thruqe/whatsrook/updater"
)

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
	// Should be true in the project repository
	if !updater.IsGitRepo() {
		t.Errorf("expected IsGitRepo to return true in current codebase")
	}
}
