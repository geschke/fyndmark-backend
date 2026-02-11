package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/geschke/fyndmark/pkg/db"
)

const DefaultQueueSize = 32

var (
	ErrQueueFull     = errors.New("pipeline queue is full")
	ErrWorkerStopped = errors.New("pipeline worker stopped")
)

type RunRequest struct {
	RunID     int64
	SiteID    string
	CommentID string
}

type Worker struct {
	db      *db.DB
	queue   chan RunRequest
	stopCh  chan struct{}
	stopped atomic.Bool
	wg      sync.WaitGroup
}

func NewWorker(database *db.DB, queueSize int) *Worker {
	if queueSize <= 0 {
		queueSize = DefaultQueueSize
	}
	return &Worker{
		db:     database,
		queue:  make(chan RunRequest, queueSize),
		stopCh: make(chan struct{}),
	}
}

func (w *Worker) Start() {
	if w == nil {
		return
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			select {
			case <-w.stopCh:
				return
			case req := <-w.queue:
				w.runOne(req)
			}
		}
	}()
}

func (w *Worker) Stop(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if w.stopped.CompareAndSwap(false, true) {
		close(w.stopCh)
	}

	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Worker) EnqueueRun(runID int64, siteID, commentID string) error {
	if w == nil {
		return ErrWorkerStopped
	}
	if w.stopped.Load() {
		return ErrWorkerStopped
	}

	req := RunRequest{
		RunID:     runID,
		SiteID:    siteID,
		CommentID: commentID,
	}

	select {
	case w.queue <- req:
		return nil
	default:
		return ErrQueueFull
	}
}

func (w *Worker) runOne(req RunRequest) {
	if w == nil || w.db == nil {
		return
	}

	runner := Runner{
		DB:      w.db,
		SiteKey: req.SiteID,
	}

	if err := runner.RunExisting(context.Background(), req.RunID); err != nil {
		_ = w.db.MarkRunFailed(req.RunID, "pipeline", fmt.Sprintf("run failed: %v", err))
	}
}
