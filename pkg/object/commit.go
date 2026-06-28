package object

import (
	"bytes"
	"encoding/binary"

	"github.com/thulasiramk-2310/vara/internal/errors"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Commit represents a single point in history.
type Commit struct {
	TreeHash  types.TreeID
	Parents   []types.CommitID
	Author    string
	Message   string
	Timestamp int64
}

// Type returns TypeCommit.
func (c *Commit) Type() ObjectType {
	return TypeCommit
}

// Serialize converts the commit to its raw on-disk byte format.
func (c *Commit) Serialize() []byte {
	var buf bytes.Buffer
	WriteHeader(&buf, TypeCommit)
	
	buf.Write(c.TreeHash[:])
	
	// Parents count + Parent hashes
	binary.Write(&buf, binary.BigEndian, uint32(len(c.Parents)))
	for _, p := range c.Parents {
		buf.Write(p[:])
	}
	
	// Timestamp
	binary.Write(&buf, binary.BigEndian, c.Timestamp)
	
	// Author + \0
	buf.WriteString(c.Author)
	buf.WriteByte(0)
	
	// Message (rest of the payload)
	buf.WriteString(c.Message)
	
	return buf.Bytes()
}

// DeserializeCommit reads a commit from the reader.
func DeserializeCommit(r *bytes.Reader) (*Commit, error) {
	c := &Commit{}
	
	if n, err := r.Read(c.TreeHash[:]); err != nil || n != 32 {
		return nil, errors.ErrInvalidObject
	}
	
	var parentCount uint32
	if err := binary.Read(r, binary.BigEndian, &parentCount); err != nil {
		return nil, errors.ErrInvalidObject
	}
	
	c.Parents = make([]types.CommitID, parentCount)
	for i := uint32(0); i < parentCount; i++ {
		if n, err := r.Read(c.Parents[i][:]); err != nil || n != 32 {
			return nil, errors.ErrInvalidObject
		}
	}
	
	if err := binary.Read(r, binary.BigEndian, &c.Timestamp); err != nil {
		return nil, errors.ErrInvalidObject
	}
	
	authorBytes, err := readUntilNul(r)
	if err != nil {
		return nil, errors.ErrInvalidObject
	}
	c.Author = string(authorBytes)
	
	var msgBuf bytes.Buffer
	msgBuf.ReadFrom(r)
	c.Message = msgBuf.String()
	
	return c, nil
}
