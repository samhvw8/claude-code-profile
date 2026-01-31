// Package source provides unified source management for Claude Code profiles.
// It combines the old skills and plugin systems into a single source abstraction
// with provider (git, http) and registry (skills.sh, github) layers.
package source

import "time"

// PackageInfo represents a package found in a registry
type PackageInfo struct {
	ID          string   `toml:"id"`          // e.g., "samhoang/debugging"
	Name        string   `toml:"name"`        // display name
	Description string   `toml:"description"`
	Registry    string   `toml:"registry"`    // which registry (skills.sh, github, manual)
	Version     string   `toml:"version"`
	Tags        []string `toml:"tags"`        // skills, agents, hooks, etc.
}

// PackageDetails extends PackageInfo with download information
type PackageDetails struct {
	PackageInfo
	DownloadURL  string   `toml:"download_url"`  // where to fetch
	ProviderType string   `toml:"provider_type"` // git, http, etc.
	Ref          string   `toml:"ref"`           // version/tag/branch
	Contents     []string `toml:"contents"`      // available items: ["skills/debugging"]
}

// Source represents an installed source in registry.toml
type Source struct {
	Registry  string    `toml:"registry"`           // which registry it came from
	Provider  string    `toml:"provider"`           // how it was downloaded (git, http)
	URL       string    `toml:"url"`                // original URL
	Path      string    `toml:"path"`               // local path in sources/
	Ref       string    `toml:"ref"`                // git ref or version
	Commit    string    `toml:"commit,omitempty"`   // git commit SHA
	Checksum  string    `toml:"checksum,omitempty"` // for http downloads
	Updated   time.Time `toml:"updated"`
	Installed []string  `toml:"installed"` // installed items
}

// FetchOptions for Provider.Fetch
type FetchOptions struct {
	Ref      string            // branch/tag for git, version for others
	Auth     *AuthConfig       // optional auth
	Headers  map[string]string // for HTTP
	Progress bool              // show progress
}

// AuthConfig for authenticated fetches
type AuthConfig struct {
	Type  string // "token", "ssh", "basic"
	Token string
}

// UpdateOptions for Provider.Update
type UpdateOptions struct {
	Ref string // new ref to update to
}
