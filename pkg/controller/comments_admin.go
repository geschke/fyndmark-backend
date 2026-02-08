package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/cors"
	"github.com/geschke/fyndmark/pkg/db"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

type CommentsAdminController struct {
	DB          *db.DB
	Store       sessions.Store
	SessionName string
	Enqueuer    PipelineEnqueuer
}

func NewCommentsAdminController(database *db.DB, store sessions.Store, sessionName string, enqueuer PipelineEnqueuer) *CommentsAdminController {
	return &CommentsAdminController{
		DB:          database,
		Store:       store,
		SessionName: sessionName,
		Enqueuer:    enqueuer,
	}
}

func (ct CommentsAdminController) Options(c *gin.Context) {
	_ = cors.ApplyCORS(c, config.Cfg.Auth.CORSAllowedOrigins)
}

func (ct CommentsAdminController) ensureAuthorized(c *gin.Context) bool {
	if ct.DB == nil || ct.DB.SQL == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "DB_NOT_INITIALIZED"})
		return false
	}
	if ct.Store == nil || strings.TrimSpace(ct.SessionName) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "AUTH_NOT_CONFIGURED"})
		return false
	}
	sess, _ := ct.Store.Get(c.Request, ct.SessionName)
	if sess == nil || sess.IsNew {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "UNAUTHORIZED"})
		return false
	}
	if _, ok := sess.Values["id"]; !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "UNAUTHORIZED"})
		return false
	}
	return true
}

type commentModerationBatchRequest struct {
	SiteID     string   `json:"SiteID"`
	CommentIDs []string `json:"CommentIDs"`
}

type commentModerationResult struct {
	CommentID string `json:"comment_id"`
	Changed   bool   `json:"changed"`
	Status    string `json:"status"`
}

// GET /api/comments/list?site_id=<id>&status=pending|approved|rejected|all&limit=..&offset=..
func (ct CommentsAdminController) GetList(c *gin.Context) {
	if !cors.ApplyCORS(c, config.Cfg.Auth.CORSAllowedOrigins) {
		return
	}
	if !ct.ensureAuthorized(c) {
		return
	}

	siteID := strings.TrimSpace(c.Query("site_id"))
	if siteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "MISSING_SITE_ID"})
		return
	}

	status := strings.ToLower(strings.TrimSpace(c.DefaultQuery("status", "pending")))
	switch status {
	case "pending", "approved", "rejected", "all":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "INVALID_STATUS"})
		return
	}

	limit := 10
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "INVALID_LIMIT"})
			return
		}
		limit = n
	}

	offset := 0
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "INVALID_OFFSET"})
			return
		}
		offset = n
	}

	filter := db.CommentListFilter{
		SiteID: siteID,
		Status: status,
		Limit:  limit,
		Offset: offset,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, err := ct.DB.CountComments(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "DB_ERROR"})
		return
	}

	list, err := ct.DB.ListComments(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "DB_ERROR"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"items":   list,
		"count":   total,
	})
}

// POST /api/comments/approve
func (ct CommentsAdminController) PostApprove(c *gin.Context) {
	ct.postModerateBatch(c, "approve")
}

// POST /api/comments/reject
func (ct CommentsAdminController) PostReject(c *gin.Context) {
	ct.postModerateBatch(c, "reject")
}

func (ct CommentsAdminController) postModerateBatch(c *gin.Context, action string) {
	if !cors.ApplyCORS(c, config.Cfg.Auth.CORSAllowedOrigins) {
		return
	}
	if !ct.ensureAuthorized(c) {
		return
	}

	var req commentModerationBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "INVALID_JSON"})
		return
	}

	siteID := strings.TrimSpace(req.SiteID)
	if siteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "MISSING_SITE_ID"})
		return
	}
	if _, ok := config.Cfg.CommentSites[siteID]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "UNKNOWN_SITE"})
		return
	}

	if len(req.CommentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "MISSING_COMMENT_IDS"})
		return
	}

	// Deduplicate and normalize IDs while preserving first-seen order.
	seen := make(map[string]struct{}, len(req.CommentIDs))
	ids := make([]string, 0, len(req.CommentIDs))
	for _, id := range req.CommentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "MISSING_COMMENT_IDS"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	results := make([]commentModerationResult, 0, len(ids))
	changedAny := false
	for _, id := range ids {
		res := commentModerationResult{
			CommentID: id,
		}

		switch action {
		case "approve":
			changed, err := ct.DB.ApproveComment(ctx, siteID, id)
			if err != nil {
				res.Changed = false
				res.Status = "error"
				results = append(results, res)
				continue
			}
			res.Changed = changed
			res.Status = "approved"
			if changed {
				changedAny = true
			}
			results = append(results, res)
		case "reject":
			changed, err := ct.DB.RejectComment(ctx, siteID, id)
			if err != nil {
				res.Changed = false
				res.Status = "error"
				results = append(results, res)
				continue
			}
			res.Changed = changed
			res.Status = "rejected"
			results = append(results, res)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "INVALID_ACTION"})
			return
		}
	}

	batchRunID := int64(0)
	batchWarning := ""
	if action == "approve" && changedAny && ct.Enqueuer != nil {
		runID, err := ct.DB.CreateRun(siteID, "")
		if err != nil {
			batchWarning = "pipeline_enqueue_failed"
		} else if err := ct.Enqueuer.EnqueueRun(runID, siteID, ""); err != nil {
			_ = ct.DB.MarkRunFailed(runID, "enqueue", err.Error())
			batchWarning = "pipeline_enqueue_failed"
		} else {
			batchRunID = runID
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"results":      results,
		"count":        len(results),
		"batch_run_id": batchRunID,
		"warning":      batchWarning,
	})
}
