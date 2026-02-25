package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type CloneOptions struct {
	RepoURL     string
	Branch      string
	AccessToken string
	TargetDir   string
	Depth       int
	Timeout     time.Duration

	RecurseSubmodules bool
}

// Clone runs: git clone [--depth=N] [--branch BRANCH] [--recurse-submodules] <url> <targetDir>
// It supports HTTPS token auth by embedding the token into the URL.
// Important: do not log args, because the URL may contain the token.
func Clone(ctx context.Context, opts CloneOptions) error {
	if strings.TrimSpace(opts.RepoURL) == "" {
		return fmt.Errorf("repo url is empty")
	}
	if strings.TrimSpace(opts.TargetDir) == "" {
		return fmt.Errorf("target dir is empty")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}

	cloneURL, err := buildHTTPSURLWithToken(opts.RepoURL, opts.AccessToken)
	if err != nil {
		return err
	}

	args := []string{"clone"}

	if opts.RecurseSubmodules {
		args = append(args, "--recurse-submodules")
	}

	if opts.Depth > 0 {
		args = append(args, fmt.Sprintf("--depth=%d", opts.Depth))
	}
	if strings.TrimSpace(opts.Branch) != "" {
		args = append(args, "--branch", opts.Branch)
	}

	args = append(args, cloneURL, opts.TargetDir)

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	_, err = runGit(runCtx, "", args)
	if err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	return nil
}

// StatusPorcelain returns the raw output of: git status --porcelain
func StatusPorcelain(ctx context.Context, repoDir string, timeout time.Duration) (string, error) {
	if strings.TrimSpace(repoDir) == "" {
		return "", fmt.Errorf("repo dir is empty")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := runGit(runCtx, repoDir, []string{"status", "--porcelain"})
	if err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}
	return out, nil
}

// AddAll stages all changes including new files: git add -A
func AddAll(ctx context.Context, repoDir string, timeout time.Duration) error {
	if strings.TrimSpace(repoDir) == "" {
		return fmt.Errorf("repo dir is empty")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := runGit(runCtx, repoDir, []string{"add", "-A"})
	if err != nil {
		return fmt.Errorf("git add -A failed: %w", err)
	}
	return nil
}

// Commit creates a commit with the given message: git commit -m "<msg>"
func Commit(ctx context.Context, repoDir string, message string, timeout time.Duration) error {
	if strings.TrimSpace(repoDir) == "" {
		return fmt.Errorf("repo dir is empty")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Errorf("commit message is empty")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := runGit(runCtx, repoDir, []string{"commit", "-m", message})
	if err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}
	return nil
}

// Push pushes to the default configured remote/branch: git push
func Push(ctx context.Context, repoDir string, timeout time.Duration) error {
	if strings.TrimSpace(repoDir) == "" {
		return fmt.Errorf("repo dir is empty")
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := runGit(runCtx, repoDir, []string{"push"})
	if err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}
	return nil
}

// runGit runs the configured operation.
func runGit(ctx context.Context, dir string, args []string) (string, error) {
	var out bytes.Buffer

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, redact(out.String()))
	}
	return out.String(), nil
}

// buildHTTPSURLWithToken performs its package-specific operation.
func buildHTTPSURLWithToken(repoURL string, token string) (string, error) {
	u := strings.TrimSpace(repoURL)
	if !strings.HasPrefix(u, "https://") {
		return "", fmt.Errorf("only https repo URLs are supported for token auth: %q", repoURL)
	}

	// Allow public clone without token.
	if strings.TrimSpace(token) == "" {
		return u, nil
	}

	// GitHub supports: https://x-access-token:TOKEN@github.com/owner/repo.git
	const prefix = "https://"
	return prefix + "x-access-token:" + token + "@" + strings.TrimPrefix(u, prefix), nil
}

// redact performs its package-specific operation.
func redact(s string) string {
	out := s
	for {
		i := strings.Index(out, "x-access-token:")
		if i < 0 {
			break
		}
		j := strings.Index(out[i:], "@")
		if j < 0 {
			break
		}
		j = i + j
		out = out[:i] + "x-access-token:***REDACTED***" + out[j:]
	}
	return out
}
