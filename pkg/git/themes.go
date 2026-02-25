package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/gitcli"
)

// ensureThemes performs its package-specific operation.
func ensureThemes(ctx context.Context, siteID string, workDir string) error {
	siteCfg, ok := config.Cfg.CommentSites[siteID]
	if !ok {
		return fmt.Errorf("unknown site_id %q (not found in comment_sites)", siteID)
	}

	themes := siteCfg.Git.Themes
	if len(themes) == 0 {
		return nil
	}

	for _, t := range themes {
		repoURL := strings.TrimSpace(t.RepoURL)
		if repoURL == "" {
			return fmt.Errorf("comment_sites.%s.git.themes: repo_url must be set", siteID)
		}

		targetRel := strings.TrimSpace(t.TargetPath)
		if targetRel == "" {
			return fmt.Errorf("comment_sites.%s.git.themes: target_path must be set", siteID)
		}

		targetRelClean, err := sanitizeRelativePath(targetRel)
		if err != nil {
			return fmt.Errorf("invalid theme target_path %q: %w", targetRel, err)
		}

		targetAbs := filepath.Join(workDir, targetRelClean)

		// If the directory already exists, assume it's present and skip.
		if existsDir(targetAbs) {
			continue
		}

		// Ensure parent directory exists.
		if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
			return fmt.Errorf("failed to create theme parent dir for %q: %w", targetAbs, err)
		}

		fmt.Printf("Cloning theme into: %s\n", targetAbs)

		if err := gitcli.Clone(ctx, gitcli.CloneOptions{
			RepoURL:     repoURL,
			Branch:      strings.TrimSpace(t.Branch),
			AccessToken: strings.TrimSpace(t.AccessToken),
			TargetDir:   targetAbs,
			Depth:       t.Depth,
			Timeout:     2 * time.Minute,
			// RecurseSubmodules intentionally not applied to theme clones by default.
		}); err != nil {
			name := strings.TrimSpace(t.Name)
			if name == "" {
				name = repoURL
			}
			return fmt.Errorf("failed to clone theme %q: %w", name, err)
		}
	}

	return nil
}

// existsDir performs its package-specific operation.
func existsDir(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	return st.IsDir()
}

// sanitizeRelativePath performs its package-specific operation.
func sanitizeRelativePath(p string) (string, error) {
	// Reject absolute paths.
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	clean := filepath.Clean(p)

	// Reject anything that escapes the repo (../...).
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository (.. is not allowed)")
	}

	// Normalize "./x" -> "x"
	clean = strings.TrimPrefix(clean, "."+string(filepath.Separator))
	if clean == "." || clean == "" {
		return "", fmt.Errorf("invalid relative path")
	}

	return clean, nil
}
