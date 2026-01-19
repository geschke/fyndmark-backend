package server

import (
	"github.com/geschke/fyntral/config"
	"github.com/geschke/fyntral/pkg/controller"

	"github.com/gin-gonic/gin"
)

func Start() error {

	gin.SetMode(gin.ReleaseMode)

	//if config.LogLevel == "debug" {
	gin.SetMode(gin.DebugMode)
	//}

	router := gin.New()
	feedback := controller.NewFeedbackController()

	// public routes
	router.GET("/", getMain)
	router.POST("/api/feedbackmail/:formid", feedback.PostMail)
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
