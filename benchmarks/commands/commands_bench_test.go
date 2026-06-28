// Benchmarks for command-level operations against the defined performance budgets.
//
// Budget targets:
//   vara init              < 20 ms
//   vara status (10k)      < 100 ms
//   vara add (1k files)    < 500 ms
//   vara commit            < 200 ms
//   vara switch            < 500 ms
//   vara history (10k)     < 100 ms
package commands_bench

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/index"
)

func newCtx(b *testing.B) (*commands.Context, string) {
	b.Helper()
	dir := b.TempDir()
	repo, err := repository.Init(dir)
	if err != nil {
		b.Fatalf("init: %v", err)
	}
	return &commands.Context{Repository: repo, Index: index.New()}, dir
}

func reloadCtx(b *testing.B, ctx *commands.Context) *commands.Context {
	b.Helper()
	idxBytes, _ := os.ReadFile(filepath.Join(ctx.Repository.VaraDir, "index"))
	var idx *index.Index
	if len(idxBytes) > 0 {
		idx, _ = index.Deserialize(idxBytes)
	} else {
		idx = index.New()
	}
	return &commands.Context{Repository: ctx.Repository, Index: idx}
}

func createFiles(b *testing.B, dir string, count int, content string) {
	b.Helper()
	for i := 0; i < count; i++ {
		if err := os.WriteFile(
			filepath.Join(dir, fmt.Sprintf("file_%04d.txt", i)),
			[]byte(content), 0644,
		); err != nil {
			b.Fatalf("write file: %v", err)
		}
	}
}

func mustAdd(b *testing.B, ctx *commands.Context) {
	b.Helper()
	if err := commands.RunAdd(ctx, []string{"."}); err != nil {
		b.Fatalf("add: %v", err)
	}
}

func mustCommit(b *testing.B, ctx *commands.Context, msg string) {
	b.Helper()
	if _, err := commands.RunCommit(ctx, msg); err != nil {
		b.Fatalf("commit: %v", err)
	}
}

// BenchmarkInit — Budget: < 20 ms
func BenchmarkInit(b *testing.B) {
	for i := 0; i < b.N; i++ {
		dir, _ := os.MkdirTemp("", "vara-bench-init-*")
		b.StartTimer()
		_, err := repository.Init(dir)
		b.StopTimer()
		if err != nil {
			b.Fatalf("init: %v", err)
		}
		os.RemoveAll(dir)
	}
}

// BenchmarkStatus10k — Budget: < 100 ms
func BenchmarkStatus10k(b *testing.B) {
	ctx, dir := newCtx(b)
	createFiles(b, dir, 10_000, "initial content")
	mustAdd(b, ctx)
	mustCommit(b, ctx, "initial")
	ctx = reloadCtx(b, ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := commands.RunStatus(ctx); err != nil {
			b.Fatalf("status: %v", err)
		}
	}
}

// BenchmarkAdd1k — Budget: < 500 ms
func BenchmarkAdd1k(b *testing.B) {
	ctx, dir := newCtx(b)
	createFiles(b, dir, 1_000, "initial content")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		createFiles(b, dir, 1_000, fmt.Sprintf("iteration %d", i))
		ctx = reloadCtx(b, ctx)
		b.StartTimer()

		mustAdd(b, ctx)
	}
}

// BenchmarkCommit — Budget: < 200 ms
func BenchmarkCommit(b *testing.B) {
	ctx, dir := newCtx(b)
	createFiles(b, dir, 100, "content")
	mustAdd(b, ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		createFiles(b, dir, 100, fmt.Sprintf("iteration %d", i))
		ctx = reloadCtx(b, ctx)
		mustAdd(b, ctx)
		b.StartTimer()

		mustCommit(b, ctx, fmt.Sprintf("commit %d", i))
	}
}

// BenchmarkSwitch — Budget: < 500 ms
func BenchmarkSwitch(b *testing.B) {
	ctx, dir := newCtx(b)
	createFiles(b, dir, 200, "content")
	mustAdd(b, ctx)
	mustCommit(b, ctx, "initial")
	ctx = reloadCtx(b, ctx)
	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		b.Fatalf("branch: %v", err)
	}

	branches := []string{"feature", "main"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx = reloadCtx(b, ctx)
		if _, err := commands.RunSwitch(ctx, branches[i%2]); err != nil {
			b.Fatalf("switch: %v", err)
		}
	}
}

// BenchmarkHistory10k — Budget: < 100 ms
// Building 10k commits and the initial graph.idx are setup-only and excluded
// from the timer. The timed loop measures steady-state: Load(graph.idx) + BFS.
func BenchmarkHistory10k(b *testing.B) {
	ctx, dir := newCtx(b)
	filePath := filepath.Join(dir, "file.txt")

	b.Log("building 10 000 commits (setup, excluded from timer)...")
	for i := 0; i < 10_000; i++ {
		os.WriteFile(filePath, []byte(fmt.Sprintf("v%d", i)), 0644)
		ctx = reloadCtx(b, ctx)
		mustAdd(b, ctx)
		mustCommit(b, ctx, fmt.Sprintf("commit %d", i))
	}

	// Pre-build graph.idx once during setup so the timed loop measures the
	// warm path: Load(one file read) + in-memory BFS — no object-store reads.
	b.Log("pre-building graph index (setup, excluded from timer)...")
	if _, err := commands.RunHistory(reloadCtx(b, ctx)); err != nil {
		b.Fatalf("pre-build history: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := commands.RunHistory(reloadCtx(b, ctx)); err != nil {
			b.Fatalf("history: %v", err)
		}
	}
}
