package server

import (
	"fmt"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/controller"
	"github.com/geschke/fyndmark/pkg/db"

	"github.com/gin-gonic/gin"
)

func Start() error {

	gin.SetMode(gin.ReleaseMode)

	//if config.LogLevel == "debug" {
	gin.SetMode(gin.DebugMode)
	//}

	database, err := db.Open(config.Cfg.SQLite.Path)
	if err != nil {
		// Hard fail: DB is required
		return fmt.Errorf("db open failed (sqlite.path=%q): %w", config.Cfg.SQLite.Path, err)
	}
	defer func() { _ = database.Close() }()

	if err := database.Migrate(); err != nil {
		// Hard fail: schema is required
		return fmt.Errorf("db migrate failed: %w", err)
	}

	router := gin.New()
	feedback := controller.NewFeedbackController()
	comments := controller.NewCommentsController(database)

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

	return router.Run(config.Cfg.Server.Listen)
}

func getMain(c *gin.Context) {
	c.Header("Access-Control-Allow-Methods", "PUT, POST, GET, DELETE, OPTIONS")
	c.JSON(200, gin.H{
		"message": "nothing here",
	})

}
