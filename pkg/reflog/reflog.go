package reflog

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Entry represents a single movement of a reference.
type Entry struct {
	OldID     types.CommitID
	NewID     types.CommitID
	Author    string
	Timestamp int64
	Message   string
}

// Manager handles reading and writing reflogs for references.
type Manager struct {
	VaraDir string
}

// NewManager creates a new reflog manager.
func NewManager(varaDir string) *Manager {
	return &Manager{VaraDir: varaDir}
}

// reflogPath returns the file path for a given reference's reflog.
func (m *Manager) reflogPath(refName string) string {
	return filepath.Join(m.VaraDir, "logs", filepath.FromSlash(refName))
}

// Append adds a new entry to the reflog of a reference.
func (m *Manager) Append(refName string, oldID, newID types.CommitID, author, message string) error {
	path := m.reflogPath(refName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	timestamp := time.Now().Unix()
	
	// Format: old_id new_id author <timestamp> \t message
	// Fallback to zeros for empty oldID (e.g. branch creation)
	oldStr := oldID.String()
	if oldID == (types.CommitID{}) {
		oldStr = strings.Repeat("0", 64)
	}

	line := fmt.Sprintf("%s %s %s %d\t%s\n", oldStr, newID.String(), author, timestamp, message)
	_, err = f.WriteString(line)
	return err
}

// Read reads the reflog for a reference, returning entries from oldest to newest.
func (m *Manager) Read(refName string) ([]Entry, error) {
	path := m.reflogPath(refName)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No reflog yet
		}
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue // skip malformed
		}

		meta := strings.Split(parts[0], " ")
		if len(meta) < 4 {
			continue // skip malformed
		}

		oldID, _ := types.ParseHex(meta[0])
		newID, _ := types.ParseHex(meta[1])
		
		// The timestamp is the last element in meta
		tsStr := meta[len(meta)-1]
		ts, _ := strconv.ParseInt(tsStr, 10, 64)

		// Author is everything in between
		author := strings.Join(meta[2:len(meta)-1], " ")

		entries = append(entries, Entry{
			OldID:     types.CommitID(oldID),
			NewID:     types.CommitID(newID),
			Author:    author,
			Timestamp: ts,
			Message:   parts[1],
		})
	}

	return entries, scanner.Err()
}
