package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/controller"
	"github.com/geschke/fyndmark/pkg/db"
	"github.com/geschke/fyndmark/pkg/pipeline"

	"github.com/gin-gonic/gin"
)

func Start(database *db.DB) error {
	gin.SetMode(gin.ReleaseMode)

	//if config.LogLevel == "debug" {
	//gin.SetMode(gin.DebugMode)
	//}

	router := gin.New()
	feedback := controller.NewFeedbackController()

	worker := pipeline.NewWorker(database, pipeline.DefaultQueueSize)
	worker.Start()
	comments := controller.NewCommentsController(database, worker)

	// public routes
	router.GET("/", getMain)
	router.POST("/api/feedbackmail/:formid", feedback.PostMail)
	router.GET("/api/comments/:siteid/decision", comments.GetDecision)

	router.POST("/api/comments/:siteid/", comments.PostComment)
	router.OPTIONS("/api/comments/:siteid/", comments.OptionsComment)

	router.POST("/api/comments/:siteid", comments.PostComment)
	router.OPTIONS("/api/comments/:siteid", comments.OptionsComment)

	// Basic health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	srv := &http.Server{
		Addr:    config.Cfg.Server.Listen,
		Handler: router,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var serveErr error
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown failed: %v", err)
	}
	if err := worker.Stop(shutdownCtx); err != nil {
		log.Printf("pipeline worker shutdown failed: %v", err)
	}

	return serveErr
}

func getMain(c *gin.Context) {
	c.Header("Access-Control-Allow-Methods", "PUT, POST, GET, DELETE, OPTIONS")
	c.JSON(200, gin.H{
		"message": "nothing here",
	})

}
