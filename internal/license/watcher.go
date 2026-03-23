package license

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"sync"
	"time"
)

const defaultPollInterval = 1 * time.Hour

// Watcher periodically checks the license key file for changes and re-validates.
// It uses polling (stat + mtime comparison) to avoid adding an fsnotify dependency
// for a file that changes rarely.
type Watcher struct {
	validator    *Validator
	path         string
	pollInterval time.Duration

	mu          sync.Mutex
	lastModTime time.Time
	lastSize    int64
}

// NewWatcher creates a file watcher for the license key at the given path.
func NewWatcher(validator *Validator, path string) *Watcher {
	return &Watcher{
		validator:    validator,
		path:         path,
		pollInterval: defaultPollInterval,
	}
}

// Start begins the polling loop. It blocks until the context is cancelled.
func (w *Watcher) Start(ctx context.Context) {
	// Snapshot the current file state.
	w.snapshot()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check()
		}
	}
}

// snapshot records the current file mtime and size without triggering a reload.
func (w *Watcher) snapshot() {
	w.mu.Lock()
	defer w.mu.Unlock()

	info, err := os.Stat(w.path)
	if err != nil {
		return
	}
	w.lastModTime = info.ModTime()
	w.lastSize = info.Size()
}

// check compares the file's current state against the last snapshot and
// triggers re-validation if the file has changed or been removed.
func (w *Watcher) check() {
	w.mu.Lock()
	defer w.mu.Unlock()

	info, err := os.Stat(w.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && !w.lastModTime.IsZero() {
			// File was removed — fall back to free tier.
			log.Printf("license: key file removed, reverting to free tier")
			w.validator.LoadAndValidate()
			w.lastModTime = time.Time{}
			w.lastSize = 0
		}
		return
	}

	if info.ModTime() != w.lastModTime || info.Size() != w.lastSize {
		log.Printf("license: key file changed, re-validating")
		w.validator.LoadAndValidate()
		w.lastModTime = info.ModTime()
		w.lastSize = info.Size()
	}
}
