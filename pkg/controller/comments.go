package controller

import (
	"log"
	"net/http"
	"strings"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/cors"
	"github.com/geschke/fyndmark/pkg/db"
	"github.com/geschke/fyndmark/pkg/turnstile"
	"github.com/gin-gonic/gin"
)

// CommentsController handles comment-related endpoints.
type CommentsController struct {
	DB *db.DB
}

func NewCommentsController(database *db.DB) *CommentsController {
	ct := CommentsController{DB: database}
	return &ct
}

type CreateCommentRequest struct {
	// site_id comes from URL param :siteid, not from JSON.
	EntryID        string `json:"entry_id"`  // optional (front matter)
	PostPath       string `json:"post_path"` // required (RelPermalink)
	ParentID       string `json:"parent_id"` // optional
	Author         string `json:"author"`    // required
	Body           string `json:"body"`      // required
	TurnstileToken string `json:"turnstile_token"`
}

type CreateCommentResponse struct {
	Success bool   `json:"success"`
	SiteID  string `json:"site_id"`
	ID      string `json:"id"`
	Status  string `json:"status"`
}

// POST /api/comments/:siteid/
func (ct CommentsController) PostComment(c *gin.Context) {
	siteID := c.Param("siteid")
	log.Println("PostComment called for site:", siteID)

	siteCfg, ok := config.Cfg.CommentSites[siteID]
	if !ok {
		log.Printf("Unknown site ID: %s", siteID)
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "unknown_site",
		})
		return
	}

	// Apply CORS based on the site's allowed origins.
	// If this returns false, the response is already handled (403 or 204).
	if !cors.ApplyCORS(c, siteCfg.CORSAllowedOrigins) {
		return
	}

	var req CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid_json",
		})
		return
	}

	// Turnstile verification (per site config)
	tsCfg := siteCfg.Turnstile
	okTS, tsErrors, err := turnstile.Validate(req.TurnstileToken, c.ClientIP(), tsCfg.SecretKey, tsCfg.Enabled)
	if err != nil {
		log.Printf("Turnstile verification error for site %s: %v", siteID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "captcha_verify_failed",
		})
		return
	}
	if !okTS {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":     false,
			"error":       "captcha_invalid",
			"error_codes": tsErrors,
		})
		return
	}

	// Minimal validation
	req.PostPath = strings.TrimSpace(req.PostPath)
	req.EntryID = strings.TrimSpace(req.EntryID)
	req.ParentID = strings.TrimSpace(req.ParentID)
	req.Author = strings.TrimSpace(req.Author)
	req.Body = strings.TrimSpace(req.Body)

	if req.PostPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "missing_post_path",
		})
		return
	}
	if req.Author == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "missing_author",
		})
		return
	}
	if req.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "missing_body",
		})
		return
	}

	// TODO: Persist to SQLite (next step).
	// For now, return a dummy ID and status.
	resp := CreateCommentResponse{
		Success: true,
		SiteID:  siteID,
		ID:      "dummy",
		Status:  "pending",
	}

	c.JSON(http.StatusCreated, resp)
}
