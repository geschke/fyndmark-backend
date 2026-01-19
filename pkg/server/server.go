package server

import (
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
		/* handle */
	}
	if err := database.Migrate(); err != nil { /* handle */
	}
	defer database.Close()

	router := gin.New()
	feedback := controller.NewFeedbackController()
	comments := controller.NewCommentsController(database)

	// public routes
	router.GET("/", getMain)
	router.POST("/api/feedbackmail/:formid", feedback.PostMail)
	router.POST("/api/comments/:siteid/", comments.PostComment)
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
