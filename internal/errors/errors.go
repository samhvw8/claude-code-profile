package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors
var (
	ErrNotInitialized    = errors.New("ccp not initialized: run 'ccp init' first")
	ErrAlreadyInitialized = errors.New("ccp already initialized")
	ErrProfileNotFound   = errors.New("profile not found")
	ErrProfileExists     = errors.New("profile already exists")
	ErrHubItemNotFound   = errors.New("hub item not found")
	ErrInvalidManifest   = errors.New("invalid profile manifest")
	ErrSymlinkBroken     = errors.New("broken symlink")
	ErrNotASymlink       = errors.New("path is not a symlink")
	ErrClaudeDirNotFound = errors.New("~/.claude directory not found")
)

// ProfileError wraps errors with profile context
type ProfileError struct {
	Profile string
	Op      string
	Err     error
}

func (e *ProfileError) Error() string {
	return fmt.Sprintf("profile %s: %s: %v", e.Profile, e.Op, e.Err)
}

func (e *ProfileError) Unwrap() error {
	return e.Err
}

// NewProfileError creates a new profile error
func NewProfileError(profile, op string, err error) *ProfileError {
	return &ProfileError{Profile: profile, Op: op, Err: err}
}

// HubError wraps errors with hub item context
type HubError struct {
	ItemType string
	ItemName string
	Op       string
	Err      error
}

func (e *HubError) Error() string {
	return fmt.Sprintf("hub %s/%s: %s: %v", e.ItemType, e.ItemName, e.Op, e.Err)
}

func (e *HubError) Unwrap() error {
	return e.Err
}

// NewHubError creates a new hub error
func NewHubError(itemType, itemName, op string, err error) *HubError {
	return &HubError{ItemType: itemType, ItemName: itemName, Op: op, Err: err}
}

// PathError wraps errors with path context
type PathError struct {
	Path string
	Op   string
	Err  error
}

func (e *PathError) Error() string {
	return fmt.Sprintf("%s: %s: %v", e.Op, e.Path, e.Err)
}

func (e *PathError) Unwrap() error {
	return e.Err
}

// NewPathError creates a new path error
func NewPathError(path, op string, err error) *PathError {
	return &PathError{Path: path, Op: op, Err: err}
}

// DriftError represents configuration drift
type DriftError struct {
	Profile string
	Issues  []string
}

func (e *DriftError) Error() string {
	return fmt.Sprintf("profile %s has %d configuration drift issues", e.Profile, len(e.Issues))
}

// NewDriftError creates a new drift error
func NewDriftError(profile string, issues []string) *DriftError {
	return &DriftError{Profile: profile, Issues: issues}
}
