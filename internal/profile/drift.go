package profile

import (
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/symlink"
)

// DriftType represents the type of configuration drift
type DriftType string

const (
	DriftMissing    DriftType = "missing"     // In manifest but not in directory
	DriftExtra      DriftType = "extra"       // In directory but not in manifest
	DriftBroken     DriftType = "broken"      // Symlink exists but is broken
	DriftMismatched DriftType = "mismatched"  // Symlink points to wrong target
	DriftHubMissing DriftType = "hub_missing" // In manifest but hub item doesn't exist
)

// DriftItem represents a single drift issue
type DriftItem struct {
	Type       DriftType
	ItemType   config.HubItemType
	ItemName   string
	Expected   string // Expected target (for mismatched)
	Actual     string // Actual target (for mismatched)
}

// DriftReport contains all drift issues for a profile
type DriftReport struct {
	Profile string
	Issues  []DriftItem
}

// HasDrift returns true if there are any issues
func (r *DriftReport) HasDrift() bool {
	return len(r.Issues) > 0
}

// IssuesByType groups issues by drift type
func (r *DriftReport) IssuesByType() map[DriftType][]DriftItem {
	result := make(map[DriftType][]DriftItem)
	for _, issue := range r.Issues {
		result[issue.Type] = append(result[issue.Type], issue)
	}
	return result
}

// FixOptions configures the fix behavior
type FixOptions struct {
	DryRun            bool
	Force             bool                                           // Auto-remove hub_missing items without confirmation
	ConfirmHubMissing func(items []DriftItem) ([]DriftItem, error) // Callback to confirm which hub_missing items to remove
}

// FixResult contains the result of a fix operation
type FixResult struct {
	Actions         []string
	ManifestUpdated bool
	RemovedItems    []DriftItem
}

// Detector detects configuration drift
type Detector struct {
	paths  *config.Paths
	symMgr *symlink.Manager
}

// NewDetector creates a new drift detector
func NewDetector(paths *config.Paths) *Detector {
	return &Detector{
		paths:  paths,
		symMgr: symlink.New(),
	}
}

// Detect checks a profile for configuration drift
func (d *Detector) Detect(profile *Profile) (*DriftReport, error) {
	report := &DriftReport{Profile: profile.Name}

	// Check each hub item type (except setting-fragments which are merged into settings.json)
	for _, itemType := range config.AllHubItemTypes() {
		if itemType == config.HubSettingFragments {
			continue
		}
		issues, err := d.detectItemTypeDrift(profile, itemType)
		if err != nil {
			return nil, err
		}
		report.Issues = append(report.Issues, issues...)
	}

	return report, nil
}

// detectItemTypeDrift checks drift for a specific item type
func (d *Detector) detectItemTypeDrift(profile *Profile, itemType config.HubItemType) ([]DriftItem, error) {
	var issues []DriftItem

	manifestItems := make(map[string]bool)
	for _, name := range profile.Manifest.GetHubItems(itemType) {
		manifestItems[name] = true
	}

	itemDir := filepath.Join(profile.Path, string(itemType))

	// Check for missing items (in manifest but not in directory)
	for name := range manifestItems {
		itemPath := filepath.Join(itemDir, name)
		hubPath := d.paths.HubItemPath(itemType, name)

		// Check if hub item exists first
		if _, err := os.Stat(hubPath); os.IsNotExist(err) {
			issues = append(issues, DriftItem{
				Type:     DriftHubMissing,
				ItemType: itemType,
				ItemName: name,
			})
			continue
		}

		info, err := d.symMgr.Info(itemPath)
		if err != nil {
			return nil, err
		}

		if !info.Exists {
			issues = append(issues, DriftItem{
				Type:     DriftMissing,
				ItemType: itemType,
				ItemName: name,
			})
			continue
		}

		if info.IsSymlink {
			// Check if symlink is broken
			if info.IsBroken {
				issues = append(issues, DriftItem{
					Type:     DriftBroken,
					ItemType: itemType,
					ItemName: name,
					Actual:   info.Target,
				})
				continue
			}

			// Check if symlink points to correct target
			expectedTarget := d.paths.HubItemPath(itemType, name)
			valid, err := d.symMgr.Validate(itemPath, expectedTarget)
			if err != nil {
				return nil, err
			}
			if !valid {
				issues = append(issues, DriftItem{
					Type:     DriftMismatched,
					ItemType: itemType,
					ItemName: name,
					Expected: expectedTarget,
					Actual:   info.Target,
				})
			}
		}
	}

	// Check for extra items (in directory but not in manifest)
	entries, err := os.ReadDir(itemDir)
	if err != nil {
		if os.IsNotExist(err) {
			return issues, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		name := entry.Name()
		if name[0] == '.' {
			continue
		}

		if !manifestItems[name] {
			issues = append(issues, DriftItem{
				Type:     DriftExtra,
				ItemType: itemType,
				ItemName: name,
			})
		}
	}

	return issues, nil
}

// Fix reconciles a profile to match its manifest
func (d *Detector) Fix(profile *Profile, report *DriftReport, opts FixOptions) (*FixResult, error) {
	result := &FixResult{}

	// Separate hub_missing issues from others
	var hubMissingIssues []DriftItem
	var otherIssues []DriftItem
	for _, issue := range report.Issues {
		if issue.Type == DriftHubMissing {
			hubMissingIssues = append(hubMissingIssues, issue)
		} else {
			otherIssues = append(otherIssues, issue)
		}
	}

	// Handle hub_missing issues
	if len(hubMissingIssues) > 0 {
		var toRemove []DriftItem

		if opts.DryRun {
			// In dry-run, just report what would happen
			for _, issue := range hubMissingIssues {
				result.Actions = append(result.Actions, "remove from manifest: "+string(issue.ItemType)+"/"+issue.ItemName+" (hub item not found)")
			}
		} else if opts.Force {
			// Auto-remove all
			toRemove = hubMissingIssues
		} else if opts.ConfirmHubMissing != nil {
			// Ask for confirmation
			confirmed, err := opts.ConfirmHubMissing(hubMissingIssues)
			if err != nil {
				return result, err
			}
			toRemove = confirmed
		}

		// Remove confirmed items from manifest
		if len(toRemove) > 0 {
			for _, issue := range toRemove {
				profile.Manifest.RemoveHubItem(issue.ItemType, issue.ItemName)
				result.Actions = append(result.Actions, "removed from manifest: "+string(issue.ItemType)+"/"+issue.ItemName)
				result.RemovedItems = append(result.RemovedItems, issue)
			}

			// Save updated manifest
			if err := profile.Manifest.SaveTOML(profile.Path); err != nil {
				return result, err
			}
			result.ManifestUpdated = true
		}
	}

	// Fix other issues
	for _, issue := range otherIssues {
		action, err := d.fixIssue(profile, issue, opts.DryRun)
		if err != nil {
			return result, err
		}
		if action != "" {
			result.Actions = append(result.Actions, action)
		}
	}

	return result, nil
}

// fixIssue fixes a single drift issue
func (d *Detector) fixIssue(profile *Profile, issue DriftItem, dryRun bool) (string, error) {
	itemPath := filepath.Join(profile.Path, string(issue.ItemType), issue.ItemName)
	hubPath := d.paths.HubItemPath(issue.ItemType, issue.ItemName)

	switch issue.Type {
	case DriftMissing:
		action := "create symlink: " + itemPath + " -> " + hubPath
		if !dryRun {
			// Ensure hub item exists
			if _, err := os.Stat(hubPath); err != nil {
				return "", err
			}
			if err := d.symMgr.Create(itemPath, hubPath); err != nil {
				return "", err
			}
		}
		return action, nil

	case DriftExtra:
		action := "remove: " + itemPath
		if !dryRun {
			if err := os.RemoveAll(itemPath); err != nil {
				return "", err
			}
		}
		return action, nil

	case DriftBroken, DriftMismatched:
		action := "recreate symlink: " + itemPath + " -> " + hubPath
		if !dryRun {
			// Remove existing
			if err := os.Remove(itemPath); err != nil && !os.IsNotExist(err) {
				return "", err
			}
			// Check hub item exists
			if _, err := os.Stat(hubPath); err != nil {
				return "", err
			}
			// Create new symlink
			if err := d.symMgr.Create(itemPath, hubPath); err != nil {
				return "", err
			}
		}
		return action, nil
	}

	return "", nil
}
