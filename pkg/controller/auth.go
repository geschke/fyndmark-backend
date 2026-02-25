package controller

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/cors"
	"github.com/geschke/fyndmark/pkg/db"
	"github.com/geschke/fyndmark/pkg/users"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

type AuthController struct {
	DB          *db.DB
	Store       sessions.Store
	SessionName string
}

// NewAuthController constructs and returns a new instance.
func NewAuthController(database *db.DB, store sessions.Store, sessionName string) *AuthController {
	return &AuthController{
		DB:          database,
		Store:       store,
		SessionName: sessionName,
	}
}

type loginRequest struct {
	Email             string `json:"email"`
	Password          string `json:"password"`
	ReturnSecureToken bool   `json:"returnSecureToken"`
}

// OptionsLogin handles the CORS preflight request.
func (ct AuthController) OptionsLogin(c *gin.Context) {
	// Allow preflight for browser-based clients.
	if !cors.ApplyCORS(c, config.Cfg.WebAdmin.CORSAllowedOrigins) {
		return
	}
}

// OptionsLogout handles the CORS preflight request.
func (ct AuthController) OptionsLogout(c *gin.Context) {
	// Allow preflight for browser-based clients.
	if !cors.ApplyCORS(c, config.Cfg.WebAdmin.CORSAllowedOrigins) {
		return
	}
}

// PostLogin performs its package-specific operation.
func (ct AuthController) PostLogin(c *gin.Context) {
	if !cors.ApplyCORS(c, config.Cfg.WebAdmin.CORSAllowedOrigins) {
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

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "INVALID_JSON"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := req.Password
	if email == "" || password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "MISSING_CREDENTIALS"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, found, err := ct.DB.GetUserByEmail(ctx, email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "DB_ERROR"})
		return
	}

	// Avoid leaking whether the email exists.
	if !found {
		// Do a tiny constant-time op to keep timing closer.
		_ = subtle.ConstantTimeCompare([]byte("a"), []byte("b"))
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "INVALID_CREDENTIALS"})
		return
	}

	ok, err := users.VerifyPassword(password, u.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "INVALID_CREDENTIALS"})
		return
	}
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "INVALID_CREDENTIALS"})
		return
	}

	sess, _ := ct.Store.Get(c.Request, ct.SessionName)
	sess.Values["id"] = u.ID
	sess.Values["email"] = u.Email
	sess.Values["firstname"] = u.FirstName
	sess.Values["lastname"] = u.LastName

	maxAgeDays := config.Cfg.WebAdmin.CookieMaxAgeDays
	if maxAgeDays <= 0 {
		maxAgeDays = 30
	}
	maxAge := maxAgeDays * 24 * 60 * 60

	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   config.Cfg.WebAdmin.CookieSecure,
		SameSite: parseSameSite(config.Cfg.WebAdmin.CookieSameSite),
	}

	if err := sess.Save(c.Request, c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "SESSION_SAVE_FAILED"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"id":        strconv.FormatInt(u.ID, 10),
		"email":     u.Email,
		"firstname": u.FirstName,
		"lastname":  u.LastName,
		"session":   "cookie",
	})
}

// PostLogout performs its package-specific operation.
func (ct AuthController) PostLogout(c *gin.Context) {
	if !cors.ApplyCORS(c, config.Cfg.WebAdmin.CORSAllowedOrigins) {
		return
	}

	if ct.Store == nil || strings.TrimSpace(ct.SessionName) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "AUTH_NOT_CONFIGURED"})
		return
	}

	sess, _ := ct.Store.Get(c.Request, ct.SessionName)
	if sess == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "LOGGED_OUT"})
		return
	}

	for k := range sess.Values {
		delete(sess.Values, k)
	}

	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   config.Cfg.WebAdmin.CookieSecure,
		SameSite: parseSameSite(config.Cfg.WebAdmin.CookieSameSite),
	}

	if err := sess.Save(c.Request, c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "SESSION_SAVE_FAILED"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "LOGGED_OUT"})
}

// parseSameSite performs its package-specific operation.
func parseSameSite(v string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	case "lax", "":
		return http.SameSiteLaxMode
	default:
		// Default to Lax, but still make the config error visible in logs later.
		return http.SameSiteLaxMode
	}
}
