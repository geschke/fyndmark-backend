package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/generator"
	"github.com/geschke/fyndmark/pkg/git"
	"github.com/geschke/fyndmark/pkg/hugo"
)

type RunStore interface {
	CreateRun(siteID, commentID string) (int64, error)
	MarkRunRunning(runID int64) error
	MarkRunStep(runID int64, step string) error
	MarkRunFailed(runID int64, step, msg string) error
	MarkRunSuccess(runID int64) error
}

const (
	StepCheckout = "checkout"
	StepGenerate = "generate"
	StepHugo     = "hugo"
	StepCommit   = "commit"
	StepPush     = "push"
)

type Runner struct {
	DB RunStore
}

func (r *Runner) Run(ctx context.Context, siteID string, triggerCommentID string) (int64, error) {
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return 0, fmt.Errorf("site_id is required (use --site-id)")
	}

	siteCfg, ok := config.Cfg.CommentSites[siteID]
	if !ok {
		return 0, fmt.Errorf("unknown site_id %q (not found in comment_sites)", siteID)
	}

	if r == nil || r.DB == nil {
		return 0, fmt.Errorf("pipeline runner: DB is nil")
	}

	runID, err := r.DB.CreateRun(siteID, triggerCommentID)
	if err != nil {
		return 0, err
	}

	if err := r.DB.MarkRunRunning(runID); err != nil {
		return runID, err
	}

	fail := func(step string, e error) error {
		_ = r.DB.MarkRunFailed(runID, step, e.Error())
		return fmt.Errorf("%s: %w", step, e)
	}

	// 1) Checkout (fresh clone)
	if err := r.DB.MarkRunStep(runID, StepCheckout); err != nil {
		return runID, err
	}
	if err := git.CheckoutWithContext(ctx, siteID); err != nil {
		return runID, fail(StepCheckout, err)
	}

	// 2) Generate markdown comment files
	if err := r.DB.MarkRunStep(runID, StepGenerate); err != nil {
		return runID, err
	}
	if err := generator.GenerateWithContext(ctx, siteID); err != nil {
		return runID, fail(StepGenerate, err)
	}

	// 3) Hugo (optional)
	if !siteCfg.Hugo.Disabled {
		if err := r.DB.MarkRunStep(runID, StepHugo); err != nil {
			return runID, err
		}
		if err := hugo.RunWithContext(ctx, siteID); err != nil {
			return runID, fail(StepHugo, err)
		}
	}

	// 4) Commit
	if err := r.DB.MarkRunStep(runID, StepCommit); err != nil {
		return runID, err
	}
	if err := git.CommitWithContext(ctx, siteID, "Update generated content"); err != nil {
		return runID, fail(StepCommit, err)
	}

	// 5) Push
	if err := r.DB.MarkRunStep(runID, StepPush); err != nil {
		return runID, err
	}
	if err := git.PushWithContext(ctx, siteID); err != nil {
		return runID, fail(StepPush, err)
	}

	if err := r.DB.MarkRunSuccess(runID); err != nil {
		return runID, err
	}

	return runID, nil
}
