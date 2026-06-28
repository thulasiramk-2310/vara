package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/repository"
)

func TestRepositoryInitAndDiscover(t *testing.T) {
	tempDir := t.TempDir()
	
	// Test Init
	repo, err := repository.Init(tempDir)
	if err != nil {
		t.Fatalf("Failed to init repository: %v", err)
	}

	// Verify layout
	expectedDirs := []string{
		"objects",
		filepath.Join("refs", "heads"),
		filepath.Join("refs", "tags"),
		filepath.Join("logs", "refs", "heads"),
		"locks",
		"journal",
		"snapshots",
	}
	
	for _, dir := range expectedDirs {
		path := filepath.Join(repo.VaraDir, dir)
		stat, err := os.Stat(path)
		if err != nil || !stat.IsDir() {
			t.Errorf("Expected directory %s does not exist", path)
		}
	}
	
	// Verify VERSION
	version, err := os.ReadFile(filepath.Join(repo.VaraDir, "VERSION"))
	if err != nil || string(version) != "1\n" {
		t.Errorf("Invalid or missing VERSION file")
	}

	// Verify HEAD
	head, err := os.ReadFile(filepath.Join(repo.VaraDir, "HEAD"))
	if err != nil || string(head) != "ref: refs/heads/main\n" {
		t.Errorf("Invalid or missing HEAD file")
	}

	// Test Discover
	subDir := filepath.Join(tempDir, "some", "nested", "dir")
	os.MkdirAll(subDir, 0755)
	
	discovered, err := repository.Discover(subDir)
	if err != nil {
		t.Fatalf("Failed to discover repository from nested dir: %v", err)
	}
	
	if discovered.RootDir != repo.RootDir {
		t.Errorf("Discover returned wrong root dir: expected %s, got %s", repo.RootDir, discovered.RootDir)
	}
}
