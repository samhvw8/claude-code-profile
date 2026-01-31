package source

import "fmt"

// SourceError represents a source-related error
type SourceError struct {
	Op     string // operation
	Source string // source identifier
	Err    error  // underlying error
}

func (e *SourceError) Error() string {
	if e.Source != "" {
		return fmt.Sprintf("%s %s: %v", e.Op, e.Source, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *SourceError) Unwrap() error {
	return e.Err
}

// Common errors
var (
	ErrSourceNotFound   = fmt.Errorf("source not found")
	ErrSourceExists     = fmt.Errorf("source already exists")
	ErrItemNotFound     = fmt.Errorf("item not found")
	ErrItemExists       = fmt.Errorf("item already exists")
	ErrProviderNotFound = fmt.Errorf("no provider for URL")
	ErrRegistryNotFound = fmt.Errorf("no registry for identifier")
	ErrFetchFailed      = fmt.Errorf("fetch failed")
	ErrUpdateFailed     = fmt.Errorf("update failed")
)
