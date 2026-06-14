package routes

import (
	"encoding/json"
	"errors"
	"os"
)

// Checkpoint is a persisted route definition for crash recovery.
type Checkpoint struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

// RecoveryFile manages the crash-recovery checkpoint on disk.
type RecoveryFile struct {
	Path string
}

func (f *RecoveryFile) Exists() bool {
	_, err := os.Stat(f.Path)
	return err == nil
}

func (f *RecoveryFile) Read() ([]Checkpoint, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var out []Checkpoint
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (f *RecoveryFile) Write(checkpoints []Checkpoint) error {
	if len(checkpoints) == 0 {
		return f.Delete()
	}
	data, err := json.Marshal(checkpoints)
	if err != nil {
		return err
	}
	return os.WriteFile(f.Path, data, 0o600)
}

func (f *RecoveryFile) Delete() error {
	err := os.Remove(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
