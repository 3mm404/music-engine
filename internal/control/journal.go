package control

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Append-only and fsynced before execution or acknowledgment. An incomplete tail
// fails closed: silently discarding it could replay a command after a power loss.
type Journal struct {
	file    *os.File
	results map[string]*Result
}
type journalEntry struct {
	ID     string  `json:"command_id"`
	Result *Result `json:"result"`
}

func OpenJournal(dir, deviceID string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	// Device IDs come from the API; encode instead of treating them as paths.
	name := fmt.Sprintf("%x.jsonl", sha256.Sum256([]byte(deviceID)))
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockJournal(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("el registro ya está en uso: %w", err)
	}
	j := &Journal{f, map[string]*Result{}}
	dec := json.NewDecoder(f)
	for {
		var e journalEntry
		err := dec.Decode(&e)
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("registro durable corrupto: %w", err)
		}
		if e.ID == "" {
			f.Close()
			return nil, errors.New("registro durable sin ID")
		}
		j.results[e.ID] = e.Result
	}
	return j, nil
}
func (j *Journal) Close() error { return j.file.Close() }
func (j *Journal) Record(id string, result *Result) error {
	if err := json.NewEncoder(j.file).Encode(journalEntry{id, result}); err != nil {
		return &PersistenceError{err}
	}
	if err := j.file.Sync(); err != nil {
		return &PersistenceError{err}
	}
	j.results[id] = result
	return nil
}

type PersistenceError struct{ Err error }

func (e *PersistenceError) Error() string {
	return "no se pudo guardar el registro durable: " + e.Err.Error()
}
func (e *PersistenceError) Unwrap() error { return e.Err }
