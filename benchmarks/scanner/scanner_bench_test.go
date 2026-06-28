package scanner_bench

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/worktree"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/scanner"
)

func setupTestRepo(b *testing.B, fileCount int) (string, *index.Index) {
	b.Helper()
	dir := b.TempDir()

	// 1. Create a dummy .varaignore
	os.WriteFile(filepath.Join(dir, ".varaignore"), []byte("*.log\nbuild/\n"), 0644)

	// 2. Create the files
	for i := 0; i < fileCount; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file_%d.txt", i))
		os.WriteFile(path, []byte("content"), 0644)
	}

	// 3. Create some ignored files just to make sure they get skipped
	os.Mkdir(filepath.Join(dir, "build"), 0755)
	for i := 0; i < fileCount/10; i++ {
		path := filepath.Join(dir, "build", fmt.Sprintf("out_%d.bin", i))
		os.WriteFile(path, []byte("bin"), 0644)
	}

	// 4. Create an empty index
	idx := index.New()

	return dir, idx
}

func runScannerBench(b *testing.B, fileCount int) {
	dir, idx := setupTestRepo(b, fileCount)
	wt, err := worktree.New(dir)
	if err != nil {
		b.Fatalf("worktree new failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := scanner.New(idx)
		_, err := s.Scan(wt)
		if err != nil {
			b.Fatalf("Scan failed: %v", err)
		}
	}
}

func BenchmarkScanner100Files(b *testing.B)   { runScannerBench(b, 100) }
func BenchmarkScanner1000Files(b *testing.B)  { runScannerBench(b, 1000) }
func BenchmarkScanner10000Files(b *testing.B) { runScannerBench(b, 10000) }
