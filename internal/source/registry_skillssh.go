package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const skillsShAPIBase = "https://skills.sh"

// SkillsShRegistry searches skills.sh for packages
type SkillsShRegistry struct {
	baseURL string
	client  *http.Client
}

func init() {
	RegisterRegistryProvider(&SkillsShRegistry{
		baseURL: skillsShAPIBase,
		client:  http.DefaultClient,
	})
}

func (r *SkillsShRegistry) Name() string {
	return "skills.sh"
}

func (r *SkillsShRegistry) CanHandle(identifier string) bool {
	if strings.HasPrefix(identifier, "skills.sh/") {
		return true
	}
	// Don't handle explicit github: prefix
	if strings.HasPrefix(identifier, "github:") {
		return false
	}
	// Default registry for owner/repo format without protocol
	if !strings.Contains(identifier, "://") && !strings.HasSuffix(identifier, ".git") {
		return true
	}
	return false
}

func (r *SkillsShRegistry) Search(ctx context.Context, query string, opts SearchOptions) ([]PackageInfo, error) {
	u := fmt.Sprintf("%s/api/v1/search?q=%s", r.baseURL, url.QueryEscape(query))
	if opts.Limit > 0 {
		u += fmt.Sprintf("&limit=%d", opts.Limit)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, &SourceError{Op: "skills.sh search", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &SourceError{Op: "skills.sh search",
			Err: fmt.Errorf("status %d", resp.StatusCode)}
	}

	var result struct {
		Packages []struct {
			Slug        string   `json:"slug"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Owner       string   `json:"owner"`
			Version     string   `json:"version"`
			Tags        []string `json:"tags"`
		} `json:"packages"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	packages := make([]PackageInfo, len(result.Packages))
	for i, p := range result.Packages {
		packages[i] = PackageInfo{
			ID:          fmt.Sprintf("%s/%s", p.Owner, p.Slug),
			Name:        p.Name,
			Description: p.Description,
			Registry:    "skills.sh",
			Version:     p.Version,
			Tags:        p.Tags,
		}
	}

	return packages, nil
}

func (r *SkillsShRegistry) Get(ctx context.Context, packageID string) (*PackageDetails, error) {
	packageID = strings.TrimPrefix(packageID, "skills.sh/")

	parts := strings.SplitN(packageID, "/", 2)
	if len(parts) != 2 {
		return nil, &SourceError{Op: "skills.sh get",
			Err: fmt.Errorf("invalid package ID: %s", packageID)}
	}
	owner, name := parts[0], parts[1]

	u := fmt.Sprintf("%s/api/v1/packages/%s/%s", r.baseURL, owner, name)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, &SourceError{Op: "skills.sh get", Source: packageID, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &SourceError{Op: "skills.sh get", Source: packageID, Err: ErrSourceNotFound}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &SourceError{Op: "skills.sh get", Source: packageID,
			Err: fmt.Errorf("status %d", resp.StatusCode)}
	}

	var result struct {
		Slug        string   `json:"slug"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Owner       string   `json:"owner"`
		Version     string   `json:"version"`
		Tags        []string `json:"tags"`
		RepoURL     string   `json:"repo_url"`
		RepoRef     string   `json:"repo_ref"`
		Contents    []string `json:"contents"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	providerType := "git"
	if !strings.Contains(result.RepoURL, "github.com") &&
		!strings.Contains(result.RepoURL, "gitlab.com") {
		if strings.HasSuffix(result.RepoURL, ".tar.gz") ||
			strings.HasSuffix(result.RepoURL, ".zip") {
			providerType = "http"
		}
	}

	return &PackageDetails{
		PackageInfo: PackageInfo{
			ID:          fmt.Sprintf("%s/%s", result.Owner, result.Slug),
			Name:        result.Name,
			Description: result.Description,
			Registry:    "skills.sh",
			Version:     result.Version,
			Tags:        result.Tags,
		},
		DownloadURL:  result.RepoURL,
		ProviderType: providerType,
		Ref:          result.RepoRef,
		Contents:     result.Contents,
	}, nil
}
