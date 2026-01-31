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
	// Add agent-skills topic by default (most common for claude skills repos)
	// User can override with their own topic: filter
	searchQuery := query
	if !strings.Contains(query, "topic:") {
		searchQuery = query + " topic:agent-skills"
	}

	perPage := 20
	if opts.Limit > 0 && opts.Limit < perPage {
		perPage = opts.Limit
	}

	// Build URL with proper encoding
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
