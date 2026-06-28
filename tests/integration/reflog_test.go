package integration

import (
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/reflog"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

func TestReflogManager(t *testing.T) {
	varaDir := t.TempDir()
	manager := reflog.NewManager(varaDir)

	id1, _ := types.ParseHex(strings.Repeat("1", 64))
	id2, _ := types.ParseHex(strings.Repeat("2", 64))
	zeroID := types.CommitID{}

	err := manager.Append("HEAD", zeroID, types.CommitID(id1), "User A", "commit: first")
	if err != nil {
		t.Fatalf("Append 1 failed: %v", err)
	}

	err = manager.Append("HEAD", types.CommitID(id1), types.CommitID(id2), "User B", "commit: second")
	if err != nil {
		t.Fatalf("Append 2 failed: %v", err)
	}

	entries, err := manager.Read("HEAD")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	if entries[0].OldID != zeroID {
		t.Errorf("Expected first OldID to be zero, got %v", entries[0].OldID)
	}
	if entries[0].NewID != types.CommitID(id1) {
		t.Errorf("Expected first NewID to be id1, got %v", entries[0].NewID)
	}
	if entries[0].Message != "commit: first" {
		t.Errorf("Expected message 'commit: first', got '%s'", entries[0].Message)
	}

	if entries[1].OldID != types.CommitID(id1) {
		t.Errorf("Expected second OldID to be id1, got %v", entries[1].OldID)
	}
	if entries[1].NewID != types.CommitID(id2) {
		t.Errorf("Expected second NewID to be id2, got %v", entries[1].NewID)
	}
	if entries[1].Message != "commit: second" {
		t.Errorf("Expected message 'commit: second', got '%s'", entries[1].Message)
	}
}
