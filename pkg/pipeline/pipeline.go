package pipeline

import (
	"context"
	"fmt"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/db"
	"github.com/geschke/fyndmark/pkg/generator"
	"github.com/geschke/fyndmark/pkg/git"
	"github.com/geschke/fyndmark/pkg/hugo"
)

const (
	StepCheckout = "checkout"
	StepGenerate = "generate"
	StepHugo     = "hugo"
	StepCommit   = "commit"
	StepPush     = "push"
)

type Runner struct {
	DB     *db.DB
	SiteID string
}

func (r *Runner) Run(ctx context.Context, triggerCommentID string) (int64, error) {

	siteCfg, ok := config.Cfg.CommentSites[r.SiteID]
	if !ok {
		return 0, fmt.Errorf("unknown site_id %q (not found in comment_sites)", r.SiteID)
	}

	if r == nil || r.DB == nil {
		return 0, fmt.Errorf("pipeline runner: DB is nil")
	}

	runID, err := r.DB.CreateRun(r.SiteID, triggerCommentID)
	if err != nil {
		return 0, err
	}

	if err := r.runWithID(ctx, runID, siteCfg); err != nil {
		return runID, err
	}

	return runID, nil
}

func (r *Runner) RunExisting(ctx context.Context, runID int64) error {
	siteCfg, ok := config.Cfg.CommentSites[r.SiteID]
	if !ok {
		return fmt.Errorf("unknown site_id %q (not found in comment_sites)", r.SiteID)
	}

	if r == nil || r.DB == nil {
		return fmt.Errorf("pipeline runner: DB is nil")
	}

	return r.runWithID(ctx, runID, siteCfg)
}

func (r *Runner) runWithID(ctx context.Context, runID int64, siteCfg config.CommentsSiteConfig) error {
	if err := r.DB.MarkRunRunning(runID); err != nil {
		return err
	}

	fail := func(step string, e error) error {
		_ = r.DB.MarkRunFailed(runID, step, e.Error())
		return fmt.Errorf("%s: %w", step, e)
	}

	// 1) Checkout (fresh clone)
	if err := r.DB.MarkRunStep(runID, StepCheckout); err != nil {
		return err
	}
	if err := git.CheckoutWithContext(ctx, r.SiteID); err != nil {
		return fail(StepCheckout, err)
	}

	// 2) Generate markdown comment files
	if err := r.DB.MarkRunStep(runID, StepGenerate); err != nil {
		return err
	}
	g := generator.Generator{
		DB:     r.DB,
		SiteID: r.SiteID,
	}
	if err := g.Generate(ctx); err != nil {
		return fail(StepGenerate, err)
	}

	// 3) Hugo (optional)
	if !siteCfg.Hugo.Disabled {
		if err := r.DB.MarkRunStep(runID, StepHugo); err != nil {
			return err
		}
		if err := hugo.RunWithContext(ctx, r.SiteID); err != nil {
			return fail(StepHugo, err)
		}
	}

	// 4) Commit
	if err := r.DB.MarkRunStep(runID, StepCommit); err != nil {
		return err
	}
	if err := git.CommitWithContext(ctx, r.SiteID, "Update generated content"); err != nil {
		return fail(StepCommit, err)
	}

	// 5) Push
	if err := r.DB.MarkRunStep(runID, StepPush); err != nil {
		return err
	}
	if err := git.PushWithContext(ctx, r.SiteID); err != nil {
		return fail(StepPush, err)
	}

	if err := r.DB.MarkRunSuccess(runID); err != nil {
		return err
	}

	return nil
}
