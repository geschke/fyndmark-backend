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
	"github.com/gorilla/sessions"
)

func Start(database *db.DB) error {
	gin.SetMode(gin.ReleaseMode)

	//if config.LogLevel == "debug" {
	gin.SetMode(gin.DebugMode)
	//}

	router := gin.New()
	feedback := controller.NewFeedbackController()

	worker := pipeline.NewWorker(database, pipeline.DefaultQueueSize)
	worker.Start()
	comments := controller.NewCommentsController(database, worker)

	if config.Cfg.Auth.Enabled {
		sessionName := config.Cfg.Auth.SessionName
		if sessionName == "" {
			sessionName = "fyndmark_session"
		}
		store := sessions.NewCookieStore([]byte(config.Cfg.Auth.SessionKey))
		auth := controller.NewAuthController(database, store, sessionName)
		router.POST("/api/auth/login", auth.PostLogin)
		router.OPTIONS("/api/auth/login", auth.OptionsLogin)
		router.POST("/api/auth/logout", auth.PostLogout)
		router.OPTIONS("/api/auth/logout", auth.OptionsLogout)

		usersCtl := controller.NewUsersController(database, store, sessionName)
		router.GET("/api/users/list", usersCtl.GetList)
		router.OPTIONS("/api/users/list", usersCtl.Options)
		router.POST("/api/users/add", usersCtl.PostAdd)
		router.OPTIONS("/api/users/add", usersCtl.Options)
		router.GET("/api/users/:id", usersCtl.GetByID)
		router.OPTIONS("/api/users/:id", usersCtl.Options)
		router.POST("/api/users/update/:id", usersCtl.PostUpdate)
		router.OPTIONS("/api/users/update/:id", usersCtl.Options)
		router.POST("/api/users/update-password/:id", usersCtl.PostUpdatePassword)
		router.OPTIONS("/api/users/update-password/:id", usersCtl.Options)
		router.POST("/api/users/delete/:id", usersCtl.PostDelete)
		router.OPTIONS("/api/users/delete/:id", usersCtl.Options)

		sitesCtl := controller.NewSitesController(database, store, sessionName)
		router.GET("/api/sites", sitesCtl.GetList)
		router.OPTIONS("/api/sites", sitesCtl.Options)

		commentsAdminCtl := controller.NewCommentsAdminController(database, store, sessionName, worker)
		router.GET("/api/comments/list", commentsAdminCtl.GetList)
		router.OPTIONS("/api/comments/list", commentsAdminCtl.Options)
		router.POST("/api/comments/approve", commentsAdminCtl.PostApprove)
		router.OPTIONS("/api/comments/approve", commentsAdminCtl.Options)
		router.POST("/api/comments/reject", commentsAdminCtl.PostReject)
		router.OPTIONS("/api/comments/reject", commentsAdminCtl.Options)
	}

	// public routes
	router.GET("/", getMain)
	router.POST("/api/feedbackmail/:formid", feedback.PostMail)
	router.GET("/api/comments/:sitekey/decision", comments.GetDecision)

	router.POST("/api/comments/:sitekey/", comments.PostComment)
	router.OPTIONS("/api/comments/:sitekey/", comments.OptionsComment)

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
