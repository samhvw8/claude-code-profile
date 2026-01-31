package source

import "context"

// RegistryProvider defines where to discover packages
type RegistryProvider interface {
	// Name returns the registry identifier (e.g., "skills.sh", "github")
	Name() string

	// Search finds packages matching query
	Search(ctx context.Context, query string, opts SearchOptions) ([]PackageInfo, error)

	// Get fetches full package details
	Get(ctx context.Context, packageID string) (*PackageDetails, error)

	// CanHandle returns true if this registry handles the identifier
	CanHandle(identifier string) bool
}

// SearchOptions for registry searches
type SearchOptions struct {
	Limit  int      // max results
	Types  []string // filter by type (skills, agents, etc.)
	SortBy string   // relevance, stars, updated
}

// registryProviders is the registry of available registries
var registryProviders = make(map[string]RegistryProvider)

// RegisterRegistryProvider adds a registry provider
func RegisterRegistryProvider(r RegistryProvider) {
	registryProviders[r.Name()] = r
}

// GetRegistryProvider returns a registry by name
func GetRegistryProvider(name string) RegistryProvider {
	return registryProviders[name]
}

// DetectRegistry auto-selects registry based on identifier
func DetectRegistry(identifier string) RegistryProvider {
	for _, r := range registryProviders {
		if r.CanHandle(identifier) {
			return r
		}
	}
	return nil
}

// DefaultRegistry returns the default registry (skills.sh)
func DefaultRegistry() RegistryProvider {
	return registryProviders["skills.sh"]
}

// AllRegistries returns all registered registries
func AllRegistries() []RegistryProvider {
	result := make([]RegistryProvider, 0, len(registryProviders))
	for _, r := range registryProviders {
		result = append(result, r)
	}
	return result
}
