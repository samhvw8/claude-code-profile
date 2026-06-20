package source

import "strings"

// gitWebHosts are the hosts whose web URLs use /blob/<ref>/ and /tree/<ref>/
// path segments to point inside a repository.
var gitWebHosts = []string{"github.com/", "gitlab.com/", "bitbucket.org/"}

// ParseGitWebURL splits a Git host "blob"/"tree" web URL (or a raw URL) into the
// clonable repo URL, the git ref, and the in-repo sub-path.
//
// It returns a non-empty subPath ONLY when the URL points inside a repo, which
// is how callers distinguish "install this specific skill" from a plain repo
// URL. A plain repo URL (no /blob/ or /tree/ segment) yields all-empty results.
//
//	https://github.com/o/r/blob/main/skills/x/SKILL.md -> https://github.com/o/r, main, skills/x/SKILL.md
//	https://github.com/o/r/tree/dev/skills/x           -> https://github.com/o/r, dev,  skills/x
//	https://raw.githubusercontent.com/o/r/main/SKILL.md -> https://github.com/o/r, main, SKILL.md
//	https://github.com/o/r                              -> "", "", ""
func ParseGitWebURL(raw string) (repoURL, ref, subPath string) {
	scheme := "https://"
	s := raw
	for _, p := range []string{"https://", "http://"} {
		if strings.HasPrefix(s, p) {
			scheme = p
			s = strings.TrimPrefix(s, p)
			break
		}
	}

	// raw.githubusercontent.com/<owner>/<repo>/<ref>/<path...>
	if rest, ok := strings.CutPrefix(s, "raw.githubusercontent.com/"); ok {
		parts := strings.SplitN(rest, "/", 4)
		if len(parts) == 4 && parts[3] != "" {
			return "https://github.com/" + parts[0] + "/" + parts[1], parts[2], parts[3]
		}
		return "", "", ""
	}

	// Normalize GitLab's "/-/blob/" and "/-/tree/" to the GitHub-style form.
	s = strings.Replace(s, "/-/", "/", 1)

	for _, marker := range []string{"/blob/", "/tree/"} {
		repoPart, after, found := strings.Cut(s, marker)
		if !found {
			continue
		}
		if !isKnownGitHost(repoPart) || strings.Count(repoPart, "/") < 2 {
			return "", "", ""
		}
		refAndPath := strings.SplitN(after, "/", 2)
		ref = refAndPath[0]
		if len(refAndPath) == 2 {
			subPath = strings.Trim(refAndPath[1], "/")
		}
		if ref == "" || subPath == "" {
			return "", "", ""
		}
		return scheme + repoPart, ref, subPath
	}

	return "", "", ""
}

func isKnownGitHost(repoPart string) bool {
	for _, h := range gitWebHosts {
		if strings.HasPrefix(repoPart, h) {
			return true
		}
	}
	return false
}
