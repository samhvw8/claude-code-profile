package source

import (
	"context"
	"fmt"
)

// ManualRegistry is a placeholder for manually added sources
type ManualRegistry struct{}

func init() {
	RegisterRegistryProvider(&ManualRegistry{})
}

func (r *ManualRegistry) Name() string {
	return "manual"
}

func (r *ManualRegistry) CanHandle(identifier string) bool {
	return false
}

func (r *ManualRegistry) Search(ctx context.Context, query string, opts SearchOptions) ([]PackageInfo, error) {
	return nil, fmt.Errorf("manual registry does not support search")
}

func (r *ManualRegistry) Get(ctx context.Context, packageID string) (*PackageDetails, error) {
	return nil, fmt.Errorf("manual registry does not support get")
}
