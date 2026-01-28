package migration

import (
	"os"
)

// Rollback tracks changes for rollback on failure
type Rollback struct {
	dirs   []string            // directories created
	moves  []struct{ src, dst string } // files/dirs moved (src is new location)
}

// NewRollback creates a new rollback tracker
func NewRollback() *Rollback {
	return &Rollback{}
}

// AddDir records a directory that was created
func (r *Rollback) AddDir(path string) {
	r.dirs = append(r.dirs, path)
}

// AddMove records a move operation
func (r *Rollback) AddMove(newPath, originalPath string) {
	r.moves = append(r.moves, struct{ src, dst string }{newPath, originalPath})
}

// Execute performs rollback, undoing changes in reverse order
func (r *Rollback) Execute() error {
	var firstErr error

	// Undo moves first (in reverse order)
	for i := len(r.moves) - 1; i >= 0; i-- {
		move := r.moves[i]
		if err := os.Rename(move.src, move.dst); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	// Remove directories (in reverse order)
	for i := len(r.dirs) - 1; i >= 0; i-- {
		if err := os.RemoveAll(r.dirs[i]); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// Clear resets the rollback tracker
func (r *Rollback) Clear() {
	r.dirs = nil
	r.moves = nil
}
