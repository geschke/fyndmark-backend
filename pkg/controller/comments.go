package controller

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/cors"
	"github.com/geschke/fyndmark/pkg/db"
	"github.com/geschke/fyndmark/pkg/generator"
	"github.com/geschke/fyndmark/pkg/mailer"
	"github.com/geschke/fyndmark/pkg/sanitize"
	"github.com/geschke/fyndmark/pkg/turnstile"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

type CommentsController struct {
	DB       *db.DB
	Enqueuer PipelineEnqueuer
}

type PipelineEnqueuer interface {
	EnqueueRun(runID int64, siteID, commentID string) error
}

func NewCommentsController(database *db.DB, enqueuer PipelineEnqueuer) *CommentsController {
	return &CommentsController{DB: database, Enqueuer: enqueuer}
}

type CreateCommentRequest struct {
	EntryID        string `json:"entry_id"`
	PostPath       string `json:"post_path"`
	ParentID       string `json:"parent_id"`
	Author         string `json:"author"`
	Email          string `json:"email"`
	AuthorUrl      string `json:"author_url"`
	Body           string `json:"body"`
	TurnstileToken string `json:"turnstile_token"`
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

	// Minimal validation + normalization
	req.EntryID = strings.TrimSpace(req.EntryID)
	req.PostPath = strings.TrimSpace(req.PostPath)
	req.ParentID = strings.TrimSpace(req.ParentID)

	// Sanitize author name (strict whitelist, UTF-8 safe)
	var authorReport sanitize.AuthorNameReport
	req.Author, authorReport = sanitize.SanitizeAuthorName(req.Author, 0)

	if req.Author == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid_author",
		})
		return
	}
	if authorReport.Changed {
		log.Printf(
			"author sanitized (site=%s): removed_ctrl=%d removed_bad=%d",
			siteID,
			authorReport.RemovedControlChars,
			authorReport.RemovedDisallowedChars,
		)
	}

	req.AuthorUrl = strings.TrimSpace(req.AuthorUrl)

	var urlReport sanitize.AuthorURLReport
	req.AuthorUrl, urlReport, err = sanitize.SanitizeAuthorURL(req.AuthorUrl, 2048)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid_author_url",
		})
		return
	}

	if urlReport.Changed {
		log.Printf("author_url sanitized (site=%s): trimmed=%t", siteID, urlReport.Trimmed)
	}

	// Validate email strictly (plain addr-spec only)
	var emailReport sanitize.EmailReport
	req.Email, emailReport, err = sanitize.SanitizeEmail(req.Email, 254)
	if err != nil {
		if emailReport.RejectedEmpty {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "missing_email",
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid_email",
		})
		return
	}

	if emailReport.Changed {
		log.Printf("email normalized (site=%s): trimmed=%t lower=%t", siteID, emailReport.Trimmed, emailReport.Lowercased)
	}

	req.Body = strings.TrimSpace(req.Body)

	if req.PostPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing_post_path"})
		return
	}
	if req.Author == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing_author"})
		return
	}

	if req.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "missing_body"})
		return
	}

	// Size limits (basic DoS protection)
	if utf8.RuneCountInString(req.Author) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "author_too_long"})
		return
	}
	if len(req.PostPath) > 512 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "post_path_too_long"})
		return
	}
	if len(req.EntryID) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "entry_id_too_long"})
		return
	}
	if len(req.Body) > 20000 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "body_too_long"})
		return
	}

	// Generate comment ID (ULID)
	entropy := ulid.Monotonic(rand.Reader, 0)
	commentID := ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()

	// Build nullable fields for DB
	entryID := sql.NullString{Valid: false}
	if req.EntryID != "" {
		entryID = sql.NullString{String: req.EntryID, Valid: true}
	}
	parentID := sql.NullString{Valid: false}
	if req.ParentID != "" {
		parentID = sql.NullString{String: req.ParentID, Valid: true}
	}
	authorUrl := sql.NullString{Valid: false}
	if req.AuthorUrl != "" {
		authorUrl = sql.NullString{String: req.AuthorUrl, Valid: true}
	}

	// Insert into DB (pending by default)
	if ct.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "db_not_initialized"})
		return
	}

	// Validate ParentID if present (must exist, same site, same post, and be approved)
	if req.ParentID != "" {
		ok, err := ct.DB.ParentExists(context.Background(), siteID, req.ParentID, req.PostPath, true)
		if err != nil {
			log.Printf("ParentExists check failed (site=%s parent=%s): %v", siteID, req.ParentID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "db_query_failed"})
			return
		}
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid_parent_id"})
			return
		}
	}

	err = ct.DB.InsertComment(context.Background(), db.Comment{
		ID:        commentID,
		SiteID:    siteID,
		EntryID:   entryID,
		PostPath:  req.PostPath,
		ParentID:  parentID,
		Status:    "pending",
		Author:    req.Author,
		Email:     req.Email,
		AuthorUrl: authorUrl,
		Body:      req.Body,
		CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		log.Printf("DB insert failed for comment %s: %v", commentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "db_insert_failed"})
		return
	}

	// Build signed approve/reject tokens (HMAC) with expiry
	exp := time.Now().Add(72 * time.Hour).Unix()
	base := baseURLFromRequest(c)

	approvePayload := fmt.Sprintf("%s|%s|approve|%d", siteID, commentID, exp)
	rejectPayload := fmt.Sprintf("%s|%s|reject|%d", siteID, commentID, exp)

	approveToken := signToken(approvePayload, siteCfg.TokenSecret)
	rejectToken := signToken(rejectPayload, siteCfg.TokenSecret)

	approveLink := fmt.Sprintf("%s/api/comments/%s/decision?token=%s", base, siteID, approveToken)
	rejectLink := fmt.Sprintf("%s/api/comments/%s/decision?token=%s", base, siteID, rejectToken)

	// Send admin email (do not fail the request if mail fails)
	subject, body, _ := generator.BuildModerationMail(generator.ModerationMailInput{
		SiteID:     siteID,
		PostPath:   req.PostPath,
		EntryID:    req.EntryID,
		ParentID:   req.ParentID,
		CommentID:  commentID,
		Author:     req.Author,
		Email:      req.Email,
		AuthorUrl:  req.AuthorUrl,
		Body:       req.Body,
		CreatedAt:  time.Now(),
		ApproveURL: approveLink,
		RejectURL:  rejectLink,
	})

	mailSent := true
	if err := mailer.SendTextMail(siteCfg.AdminRecipients, subject, body); err != nil {
		mailSent = false
		log.Printf("Failed to send admin mail for comment %s: %v", commentID, err)
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":   true,
		"site_id":   siteID,
		"id":        commentID,
		"status":    "pending",
		"mail_sent": mailSent,
	})
}

// OPTIONS /api/comments/:siteid/
func (ct CommentsController) OptionsComment(c *gin.Context) {
	siteID := c.Param("siteid")

	siteCfg, ok := config.Cfg.CommentSites[siteID]
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	// Apply CORS for this site and finish preflight
	if !cors.ApplyCORS(c, siteCfg.CORSAllowedOrigins) {
		return
	}

	c.Status(http.StatusNoContent)
}

func signToken(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)

	p := base64.RawURLEncoding.EncodeToString([]byte(payload))
	s := base64.RawURLEncoding.EncodeToString(sig)
	return p + "." + s
}

func baseURLFromRequest(c *gin.Context) string {
	// Prefer reverse proxy headers if present.
	proto := c.GetHeader("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
	}
	host := c.Request.Host
	return proto + "://" + host
}

// GET /api/comments/:siteid/decision?token=...
func (ct CommentsController) GetDecision(c *gin.Context) {
	siteID := c.Param("siteid")

	siteCfg, ok := config.Cfg.CommentSites[siteID]
	if !ok {
		c.String(http.StatusNotFound, "unknown site")
		return
	}

	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		c.String(http.StatusBadRequest, "missing token")
		return
	}

	if ct.DB == nil || ct.DB.SQL == nil {
		c.String(http.StatusInternalServerError, "db not initialized")
		return
	}

	// token format: base64url(payload) + "." + base64url(signature)
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		c.String(http.StatusBadRequest, "invalid token format")
		return
	}

	payloadB, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		c.String(http.StatusBadRequest, "invalid token payload encoding")
		return
	}
	sigB, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		c.String(http.StatusBadRequest, "invalid token signature encoding")
		return
	}

	payload := string(payloadB)

	// Verify signature (constant-time)
	mac := hmac.New(sha256.New, []byte(siteCfg.TokenSecret))
	mac.Write([]byte(payload))
	expectedSig := mac.Sum(nil)
	if !hmac.Equal(sigB, expectedSig) {
		c.String(http.StatusForbidden, "invalid token signature")
		return
	}

	// payload format: site_id|comment_id|action|exp_unix
	fields := strings.Split(payload, "|")
	if len(fields) != 4 {
		c.String(http.StatusBadRequest, "invalid token payload")
		return
	}

	tokenSiteID := fields[0]
	commentID := fields[1]
	action := fields[2]
	expStr := fields[3]

	if tokenSiteID != siteID {
		c.String(http.StatusForbidden, "site mismatch")
		return
	}

	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid token expiry")
		return
	}

	now := time.Now().Unix()
	if now > exp {
		c.String(http.StatusForbidden, "token expired")
		return
	}

	ctx := context.Background()

	switch action {
	case "approve":
		changed, err := ct.DB.ApproveComment(ctx, siteID, commentID)
		if err != nil {
			log.Printf("approve failed (site=%s id=%s): %v", siteID, commentID, err)
			c.String(http.StatusInternalServerError, "db update failed")
			return
		}
		if !changed {
			c.String(http.StatusOK, "nothing to approve (already decided or not found)")
			return
		}

		if ct.Enqueuer == nil {
			c.String(http.StatusOK, "approved (pipeline not configured)")
			return
		}

		runID, err := ct.DB.CreateRun(siteID, commentID)
		if err != nil {
			log.Printf("create run failed (site=%s id=%s): %v", siteID, commentID, err)
			c.String(http.StatusOK, "approved (pipeline enqueue failed)")
			return
		}

		if err := ct.Enqueuer.EnqueueRun(runID, siteID, commentID); err != nil {
			_ = ct.DB.MarkRunFailed(runID, "enqueue", err.Error())
			log.Printf("enqueue run failed (site=%s id=%s run_id=%d): %v", siteID, commentID, runID, err)
			c.String(http.StatusOK, "approved (pipeline enqueue failed)")
			return
		}

		c.String(http.StatusOK, fmt.Sprintf("approved (pipeline queued, run_id=%d)", runID))
		return

	case "reject":
		changed, err := ct.DB.RejectComment(ctx, siteID, commentID)
		if err != nil {
			log.Printf("reject failed (site=%s id=%s): %v", siteID, commentID, err)
			c.String(http.StatusInternalServerError, "db update failed")
			return
		}
		if !changed {
			c.String(http.StatusOK, "nothing to reject (already decided or not found)")
			return
		}
		c.String(http.StatusOK, "rejected")
		return

	default:
		c.String(http.StatusBadRequest, "invalid action")
		return
	}

}
