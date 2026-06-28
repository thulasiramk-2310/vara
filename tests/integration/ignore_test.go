package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/worktree"
	"github.com/thulasiramk-2310/vara/pkg/ignore"
)

func TestIgnoreRules(t *testing.T) {
	// Create a temporary .varaignore file
	tmpDir := t.TempDir()
	ignorePath := filepath.Join(tmpDir, ".varaignore")
	rules := `
# A comment
*.log
build/
secret.txt
`
	if err := os.WriteFile(ignorePath, []byte(rules), 0644); err != nil {
		t.Fatalf("Failed to write .varaignore: %v", err)
	}

	matcher, err := ignore.Load(ignorePath)
	if err != nil {
		t.Fatalf("Failed to load ignore file: %v", err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{".vara", true},                     // Always ignored
		{".vara/objects/123", true},         // Always ignored
		{"app.log", true},                   // *.log match
		{"logs/server.log", true},           // *.log match
		{"build", true},                     // build/ exact directory match
		{"build/output.bin", true},          // build/ child match
		{"src/build/output.bin", true},      // directory anywhere in path
		{"secret.txt", true},                // exact match
		{"src/secret.txt", true},            // exact match anywhere
		{"main.go", false},                  // not ignored
		{"src/main.go", false},              // not ignored
	}

	for _, tt := range tests {
		got := matcher.Ignore(tt.path)
		if got != tt.want {
			t.Errorf("Ignore(%q) = %v; want %v", tt.path, got, tt.want)
		}
	}
}

func TestWorktreeWalk(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create files
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "app.log"), []byte("log"), 0644)
	
	os.Mkdir(filepath.Join(tmpDir, "build"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "build", "out.bin"), []byte("bin"), 0644)

	os.Mkdir(filepath.Join(tmpDir, ".vara"), 0755)
	os.WriteFile(filepath.Join(tmpDir, ".vara", "HEAD"), []byte("ref: refs/heads/main"), 0644)

	// Create .varaignore
	os.WriteFile(filepath.Join(tmpDir, ".varaignore"), []byte("*.log\nbuild/\n"), 0644)

	wt, err := worktree.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to init worktree: %v", err)
	}

	var walked []string
	err = wt.Walk(func(relPath string, info os.FileInfo) error {
		walked = append(walked, relPath)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	// We expect:
	// - .varaignore
	// - main.go
	// We DO NOT expect:
	// - app.log
	// - build/out.bin
	// - .vara/HEAD

	expectedFiles := map[string]bool{
		"main.go":     true,
		".varaignore": true,
	}

	if len(walked) != len(expectedFiles) {
		t.Errorf("Expected %d files, walked %d: %v", len(expectedFiles), len(walked), walked)
	}

	for _, p := range walked {
		if !expectedFiles[p] {
			t.Errorf("Unexpectedly walked file: %s", p)
		}
	}
}
