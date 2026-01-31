package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const githubAPIBase = "https://api.github.com"

// GitHubRegistry searches GitHub for claude-related repos
type GitHubRegistry struct {
	baseURL string
	client  *http.Client
	token   string
}

func init() {
	RegisterRegistryProvider(&GitHubRegistry{
		baseURL: githubAPIBase,
		client:  http.DefaultClient,
	})
}

func (r *GitHubRegistry) Name() string {
	return "github"
}

func (r *GitHubRegistry) CanHandle(identifier string) bool {
	return strings.HasPrefix(identifier, "github:")
}

func (r *GitHubRegistry) Search(ctx context.Context, query string, opts SearchOptions) ([]PackageInfo, error) {
	// Search each topic separately and merge results
	topics := []string{
		"agent-skills",
		"claude-code",
		"claude-skills",
		"awesome-skills",
		"claude-plugin",
	}

	seen := make(map[string]bool)
	var packages []PackageInfo

	for _, topic := range topics {
		searchQuery := fmt.Sprintf("%s topic:%s", query, topic)
		results, err := r.doSearch(ctx, searchQuery, 10)
		if err != nil {
			continue
		}
		for _, pkg := range results {
			if !seen[pkg.ID] {
				seen[pkg.ID] = true
				packages = append(packages, pkg)
			}
		}
	}

	// Limit results
	limit := 20
	if opts.Limit > 0 {
		limit = opts.Limit
	}
	if len(packages) > limit {
		packages = packages[:limit]
	}

	return packages, nil
}

func (r *GitHubRegistry) doSearch(ctx context.Context, searchQuery string, perPage int) ([]PackageInfo, error) {
	baseURL, _ := url.Parse(fmt.Sprintf("%s/search/repositories", r.baseURL))
	q := baseURL.Query()
	q.Set("q", searchQuery)
	q.Set("sort", "stars")
	q.Set("per_page", fmt.Sprintf("%d", perPage))
	baseURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if r.token != "" {
		req.Header.Set("Authorization", "token "+r.token)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, &SourceError{Op: "github search", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &SourceError{Op: "github search",
			Err: fmt.Errorf("status %d", resp.StatusCode)}
	}

	var result struct {
		Items []struct {
			FullName    string   `json:"full_name"`
			Description string   `json:"description"`
			Topics      []string `json:"topics"`
			HTMLURL     string   `json:"html_url"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	packages := make([]PackageInfo, len(result.Items))
	for i, item := range result.Items {
		packages[i] = PackageInfo{
			ID:          item.FullName,
			Name:        item.FullName,
			Description: item.Description,
			Registry:    "github",
			Tags:        item.Topics,
		}
	}

	return packages, nil
}

func (r *GitHubRegistry) Get(ctx context.Context, packageID string) (*PackageDetails, error) {
	packageID = strings.TrimPrefix(packageID, "github:")

	u := fmt.Sprintf("%s/repos/%s", r.baseURL, packageID)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if r.token != "" {
		req.Header.Set("Authorization", "token "+r.token)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, &SourceError{Op: "github get", Source: packageID, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &SourceError{Op: "github get", Source: packageID, Err: ErrSourceNotFound}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &SourceError{Op: "github get", Source: packageID,
			Err: fmt.Errorf("status %d", resp.StatusCode)}
	}

	var result struct {
		FullName      string   `json:"full_name"`
		Description   string   `json:"description"`
		Topics        []string `json:"topics"`
		HTMLURL       string   `json:"html_url"`
		CloneURL      string   `json:"clone_url"`
		DefaultBranch string   `json:"default_branch"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &PackageDetails{
		PackageInfo: PackageInfo{
			ID:          result.FullName,
			Name:        result.FullName,
			Description: result.Description,
			Registry:    "github",
			Tags:        result.Topics,
		},
		DownloadURL:  result.CloneURL,
		ProviderType: "git",
		Ref:          result.DefaultBranch,
	}, nil
}
