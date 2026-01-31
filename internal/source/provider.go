package source

import "context"

// Provider defines how to fetch and update sources
type Provider interface {
	// Type returns the provider identifier (e.g., "git", "http")
	Type() string

	// Fetch downloads/clones the source to destPath
	Fetch(ctx context.Context, url string, destPath string, opts FetchOptions) error

	// Update refreshes an existing source at sourcePath
	Update(ctx context.Context, sourcePath string, opts UpdateOptions) (*UpdateResult, error)

	// CanHandle returns true if this provider can handle the given URL
	CanHandle(url string) bool
}

// UpdateResult contains information about an update
type UpdateResult struct {
	Updated   bool   // true if changes were pulled
	OldCommit string // previous commit (for git)
	NewCommit string // new commit (for git)
}

// providers is the registry of available providers
var providers = make(map[string]Provider)

// RegisterProvider adds a provider to the registry
func RegisterProvider(p Provider) {
	providers[p.Type()] = p
}

// GetProvider returns a provider by type
func GetProvider(providerType string) Provider {
	return providers[providerType]
}

// DetectProvider auto-selects provider based on URL
func DetectProvider(url string) Provider {
	for _, p := range providers {
		if p.CanHandle(url) {
			return p
		}
	}
	return nil
}

// AllProviders returns all registered providers
func AllProviders() []Provider {
	result := make([]Provider, 0, len(providers))
	for _, p := range providers {
		result = append(result, p)
	}
	return result
}
