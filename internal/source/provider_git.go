package source

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// GitProvider handles git clone and pull operations
type GitProvider struct{}

func init() {
	RegisterProvider(&GitProvider{})
}

func (p *GitProvider) Type() string {
	return "git"
}

func (p *GitProvider) CanHandle(url string) bool {
	patterns := []string{
		"github.com/",
		"gitlab.com/",
		"bitbucket.org/",
		".git",
		"git@",
		"git://",
	}
	for _, pattern := range patterns {
		if strings.Contains(url, pattern) {
			return true
		}
	}
	return false
}

func (p *GitProvider) Fetch(ctx context.Context, url string, destPath string, opts FetchOptions) error {
	url = normalizeGitURL(url)

	args := []string{"clone", "--depth", "1"}
	if opts.Ref != "" {
		args = append(args, "--branch", opts.Ref)
	}
	args = append(args, url, destPath)

	cmd := exec.CommandContext(ctx, "git", args...)
	if !opts.Progress {
		cmd.Stdout = nil
		cmd.Stderr = nil
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return &SourceError{Op: "git clone", Source: url, Err: err}
	}
	return nil
}

func (p *GitProvider) Update(ctx context.Context, sourcePath string, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}
	result.OldCommit = p.getCommit(sourcePath)

	fetchCmd := exec.CommandContext(ctx, "git", "-C", sourcePath, "fetch", "origin")
	if err := fetchCmd.Run(); err != nil {
		return nil, &SourceError{Op: "git fetch", Source: sourcePath, Err: err}
	}

	ref := opts.Ref
	if ref == "" {
		ref = "origin/HEAD"
	}

	resetCmd := exec.CommandContext(ctx, "git", "-C", sourcePath, "reset", "--hard", ref)
	if err := resetCmd.Run(); err != nil {
		return nil, &SourceError{Op: "git reset", Source: sourcePath, Err: err}
	}

	result.NewCommit = p.getCommit(sourcePath)
	result.Updated = result.OldCommit != result.NewCommit
	return result, nil
}

func (p *GitProvider) getCommit(repoPath string) string {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetCommit returns the current commit SHA of a git repo
func (p *GitProvider) GetCommit(repoPath string) string {
	return p.getCommit(repoPath)
}

// GetRef returns the current branch/tag of a git repo
func (p *GitProvider) GetRef(repoPath string) string {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// normalizeGitURL converts various URL formats to HTTPS
func normalizeGitURL(url string) string {
	url = strings.TrimSuffix(url, ".git")

	if strings.HasPrefix(url, "github.com/") ||
		strings.HasPrefix(url, "gitlab.com/") ||
		strings.HasPrefix(url, "bitbucket.org/") {
		url = "https://" + url
	}

	if !strings.HasSuffix(url, ".git") {
		url = url + ".git"
	}

	return url
}
