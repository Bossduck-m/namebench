package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

var errAnotherInstanceRunning = errors.New("another instance is already running")
var instanceStateDirOverride string

type instanceState struct {
	PID       int       `json:"pid"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type instanceGuard struct {
	lock      *flock.Flock
	statePath string
}

func acquireInstanceGuard() (*instanceGuard, *instanceState, error) {
	dir, err := instanceStateDir()
	if err != nil {
		return nil, nil, err
	}

	lock := flock.New(filepath.Join(dir, "instance.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, nil, err
	}
	if !locked {
		state, readErr := readInstanceState(filepath.Join(dir, "instance.json"))
		if readErr != nil {
			return nil, nil, errAnotherInstanceRunning
		}
		return nil, state, errAnotherInstanceRunning
	}

	return &instanceGuard{
		lock:      lock,
		statePath: filepath.Join(dir, "instance.json"),
	}, nil, nil
}

func (g *instanceGuard) WriteState(state instanceState) error {
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(g.statePath, payload, 0o600)
}

func (g *instanceGuard) Release() {
	if g == nil {
		return
	}
	if g.statePath != "" {
		_ = os.Remove(g.statePath)
	}
	if g.lock != nil {
		_ = g.lock.Unlock()
	}
}

func readInstanceState(path string) (*instanceState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state instanceState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func instanceStateDir() (string, error) {
	if strings.TrimSpace(instanceStateDirOverride) != "" {
		if err := os.MkdirAll(instanceStateDirOverride, 0o755); err != nil {
			return "", err
		}
		return instanceStateDirOverride, nil
	}

	root, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(root) == "" {
		root = os.TempDir()
	}
	dir := filepath.Join(root, "namebench")
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return "", mkErr
	}
	return dir, nil
}
