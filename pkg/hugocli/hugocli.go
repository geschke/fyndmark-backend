package hugocli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type RunOptions struct {
	// WorkingDir is the Hugo site directory (repo root).
	WorkingDir string

	// HugoBin is the binary name or full path. If empty, "hugo" is used.
	HugoBin string

	// Args are additional hugo args, e.g. []string{"--minify"}.
	Args []string

	// Timeout is the maximum runtime. If <= 0, a default is used.
	Timeout time.Duration
}

// Run executes Hugo in the given WorkingDir.
// It captures stdout/stderr and returns a readable error on failures.
func Run(ctx context.Context, opts RunOptions) error {
	if strings.TrimSpace(opts.WorkingDir) == "" {
		return fmt.Errorf("working dir is empty")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}

	bin := strings.TrimSpace(opts.HugoBin)
	if bin == "" {
		bin = "hugo"
	}

	args := make([]string, 0, len(opts.Args))
	args = append(args, opts.Args...)

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	var out bytes.Buffer
	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Dir = opts.WorkingDir
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hugo failed: %w: %s", err, out.String())
	}

	return nil
}
