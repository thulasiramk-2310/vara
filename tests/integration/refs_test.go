package integration

import (

	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

func TestFSResolver(t *testing.T) {
	varaDir := t.TempDir()
	resolver := refs.NewFSResolver(varaDir)

	// Create a dummy commit ID
	dummyID, _ := types.ParseHex("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	// 1. Create a reference
	err := resolver.Create("refs/heads/main", types.CommitID(dummyID))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Ensure it exists
	id, err := resolver.Resolve("refs/heads/main")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if id != types.CommitID(dummyID) {
		t.Fatalf("Expected %v, got %v", dummyID, id)
	}

	// 2. Symbolic Reference
	err = resolver.Symbolic("HEAD", "refs/heads/main")
	if err != nil {
		t.Fatalf("Symbolic failed: %v", err)
	}

	symTarget, err := resolver.ResolveSymbolic("HEAD")
	if err != nil {
		t.Fatalf("ResolveSymbolic failed: %v", err)
	}
	if symTarget != "refs/heads/main" {
		t.Fatalf("Expected 'refs/heads/main', got %v", symTarget)
	}

	// Resolving HEAD should follow the symbolic link
	id, err = resolver.Resolve("HEAD")
	if err != nil {
		t.Fatalf("Resolve HEAD failed: %v", err)
	}
	if id != types.CommitID(dummyID) {
		t.Fatalf("Expected HEAD to resolve to %v, got %v", dummyID, id)
	}

	// Detached test
	detached, err := resolver.Detached("HEAD")
	if err != nil {
		t.Fatalf("Detached HEAD failed: %v", err)
	}
	if detached {
		t.Fatalf("HEAD should not be detached")
	}

	detached, err = resolver.Detached("refs/heads/main")
	if err != nil {
		t.Fatalf("Detached refs/heads/main failed: %v", err)
	}
	if !detached {
		t.Fatalf("refs/heads/main should be detached (direct)")
	}

	// 3. Update Reference
	dummyID2, _ := types.ParseHex("abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	err = resolver.Update("HEAD", types.CommitID(dummyID2))
	if err != nil {
		t.Fatalf("Update through HEAD failed: %v", err)
	}

	// HEAD should still point to main
	symTarget, _ = resolver.ResolveSymbolic("HEAD")
	if symTarget != "refs/heads/main" {
		t.Fatalf("Update should not overwrite symbolic link")
	}

	// But main should be updated
	id, _ = resolver.Resolve("refs/heads/main")
	if id != types.CommitID(dummyID2) {
		t.Fatalf("Expected main to be updated to %v, got %v", dummyID2, id)
	}

	// 4. List References
	// Create another one
	resolver.Create("refs/tags/v1.0", types.CommitID(dummyID))
	list, err := resolver.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	
	if len(list) != 2 {
		t.Fatalf("Expected 2 refs in list, got %d", len(list))
	}
	
	names := map[string]bool{}
	for _, r := range list {
		names[filepath.ToSlash(r.Name)] = true
	}
	if !names["refs/heads/main"] || !names["refs/tags/v1.0"] {
		t.Fatalf("List missing expected refs: %v", list)
	}
}
