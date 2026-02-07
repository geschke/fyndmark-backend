package controller

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/cors"
	"github.com/geschke/fyndmark/pkg/db"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

type UsersController struct {
	DB          *db.DB
	Store       sessions.Store
	SessionName string
}

func NewUsersController(database *db.DB, store sessions.Store, sessionName string) *UsersController {
	return &UsersController{
		DB:          database,
		Store:       store,
		SessionName: sessionName,
	}
}

func (ct UsersController) OptionsList(c *gin.Context) {
	// Allow preflight for browser-based clients.
	_ = cors.ApplyCORS(c, config.Cfg.Auth.CORSAllowedOrigins)
}

// GET /api/users/list
func (ct UsersController) GetList(c *gin.Context) {
	if !cors.ApplyCORS(c, config.Cfg.Auth.CORSAllowedOrigins) {
		return
	}

	if ct.DB == nil || ct.DB.SQL == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "DB_NOT_INITIALIZED"})
		return
	}
	if ct.Store == nil || strings.TrimSpace(ct.SessionName) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "AUTH_NOT_CONFIGURED"})
		return
	}

	sess, _ := ct.Store.Get(c.Request, ct.SessionName)
	if sess == nil || sess.IsNew {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "UNAUTHORIZED"})
		return
	}
	if _, ok := sess.Values["id"]; !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "UNAUTHORIZED"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	items, err := ct.DB.ListUsers(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "DB_ERROR"})
		return
	}

	for i := range items {
		items[i].Password = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"items":   items,
	})
}
