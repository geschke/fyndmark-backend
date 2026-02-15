package controller

import (
	"context"
	"fmt"
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

type commentModerationItem struct {
	SiteID    int64  `json:"SiteID"`
	CommentID string `json:"CommentID"`
}

type commentModerationBatchRequest struct {
	Items []commentModerationItem `json:"Items"`
}

type commentModerationResult struct {
	SiteID    int64  `json:"site_id"`
	CommentID string `json:"comment_id"`
	Changed   bool   `json:"changed"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
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

func (ct CommentsAdminController) currentSessionUserID(c *gin.Context) (int64, bool) {
	sess, _ := ct.Store.Get(c.Request, ct.SessionName)
	if sess == nil {
		return 0, false
	}
	raw, ok := sess.Values["id"]
	if !ok {
		return 0, false
	}
	id, ok := raw.(int64)
	if !ok {
		return 0, false
	}
	return id, true
}

// GET /api/comments/list?site_id=<id>&status=pending|approved|rejected|spam|deleted|all&limit=..&offset=..
func (ct CommentsAdminController) GetList(c *gin.Context) {
	if !cors.ApplyCORS(c, config.Cfg.Auth.CORSAllowedOrigins) {
		return
	}
	if !ct.ensureAuthorized(c) {
		return
	}

	siteID := int64(0)
	if v := strings.TrimSpace(c.Query("site_id")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "INVALID_SITE_ID"})
			return
		}
		siteID = n
	}
	fmt.Println("siteid in GetList", siteID)
	status := strings.ToLower(strings.TrimSpace(c.DefaultQuery("status", "pending")))
	switch status {
	case "pending", "approved", "rejected", "spam", "deleted", "all":
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userID, ok := ct.currentSessionUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "UNAUTHORIZED"})
		return
	}

	allowedSiteIDs, err := ct.DB.ListAllowedSiteIDsByUserID(ctx, userID)
	if err != nil {
		fmt.Println("1", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "DB_ERROR"})
		return
	}
	if len(allowedSiteIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "items": []db.Comment{}, "count": int64(0)})
		return
	}

	if siteID > 0 {
		hasAccess, err := ct.DB.UserHasSiteAccess(ctx, userID, siteID)
		if err != nil {
			fmt.Println("2", err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "DB_ERROR"})
			return
		}
		if !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "FORBIDDEN_SITE"})
			return
		}
	}

	filter := db.CommentListFilter{
		SiteID:         siteID,
		AllowedSiteIDs: allowedSiteIDs,
		Status:         status,
		Limit:          limit,
		Offset:         offset,
	}

	total, err := ct.DB.CountComments(ctx, filter)
	if err != nil {
		fmt.Println("3", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "DB_ERROR"})
		return
	}

	list, err := ct.DB.ListComments(ctx, filter)
	if err != nil {
		fmt.Println("4", err)
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

// POST /api/comments/spam
func (ct CommentsAdminController) PostSpam(c *gin.Context) {
	ct.postModerateBatch(c, "spam")
}

// POST /api/comments/delete
func (ct CommentsAdminController) PostDelete(c *gin.Context) {
	ct.postModerateBatch(c, "delete")
}

func (ct CommentsAdminController) postModerateBatch(c *gin.Context, action string) {
	if !cors.ApplyCORS(c, config.Cfg.Auth.CORSAllowedOrigins) {
		return
	}
	if !ct.ensureAuthorized(c) {
		return
	}
	switch action {
	case "approve", "reject", "spam", "delete":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "INVALID_ACTION"})
		return
	}

	var req commentModerationBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "INVALID_JSON"})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "MISSING_ITEMS"})
		return
	}

	userID, ok := ct.currentSessionUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "UNAUTHORIZED"})
		return
	}

	authCtx, authCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer authCancel()

	allowedSiteIDs, err := ct.DB.ListAllowedSiteIDsByUserID(authCtx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "DB_ERROR"})
		return
	}
	allowedSet := make(map[int64]struct{}, len(allowedSiteIDs))
	for _, sid := range allowedSiteIDs {
		if sid <= 0 {
			continue
		}
		allowedSet[sid] = struct{}{}
	}

	seen := make(map[string]struct{}, len(req.Items))
	items := make([]commentModerationItem, 0, len(req.Items))
	for _, item := range req.Items {
		item.CommentID = strings.TrimSpace(item.CommentID)
		if item.SiteID <= 0 || item.CommentID == "" {
			continue
		}
		key := fmt.Sprintf("%d:%s", item.SiteID, item.CommentID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}
	if len(items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "MISSING_ITEMS"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	results := make([]commentModerationResult, 0, len(items))
	approvedChangedSites := make(map[int64]struct{})
	for _, item := range items {
		res := commentModerationResult{
			SiteID:    item.SiteID,
			CommentID: item.CommentID,
		}

		if _, hasAccess := allowedSet[item.SiteID]; !hasAccess {
			res.Status = "error"
			res.Error = "FORBIDDEN_SITE"
			results = append(results, res)
			continue
		}

		switch action {
		case "approve":
			changed, err := ct.DB.ApproveComment(ctx, item.SiteID, item.CommentID)
			if err != nil {
				res.Status = "error"
				res.Error = "DB_ERROR"
				results = append(results, res)
				continue
			}
			res.Changed = changed
			res.Status = "approved"
			if changed {
				approvedChangedSites[item.SiteID] = struct{}{}
			}
			results = append(results, res)
		case "reject":
			changed, err := ct.DB.RejectComment(ctx, item.SiteID, item.CommentID)
			if err != nil {
				res.Status = "error"
				res.Error = "DB_ERROR"
				results = append(results, res)
				continue
			}
			res.Changed = changed
			res.Status = "rejected"
			results = append(results, res)
		case "spam":
			changed, err := ct.DB.SpamComment(ctx, item.SiteID, item.CommentID)
			if err != nil {
				res.Status = "error"
				res.Error = "DB_ERROR"
				results = append(results, res)
				continue
			}
			res.Changed = changed
			res.Status = "spam"
			results = append(results, res)
		case "delete":
			changed, err := ct.DB.DeleteComment(ctx, item.SiteID, item.CommentID)
			if err != nil {
				res.Status = "error"
				res.Error = "DB_ERROR"
				results = append(results, res)
				continue
			}
			res.Changed = changed
			res.Status = "deleted"
			results = append(results, res)
		}
	}

	batchRunIDs := map[string]int64{}
	warnings := map[string]string{}
	if action == "approve" && ct.Enqueuer != nil {
		for siteID := range approvedChangedSites {
			key := strconv.FormatInt(siteID, 10)
			site, found, err := ct.DB.GetSiteByID(ctx, siteID)
			if err != nil || !found {
				warnings[key] = "pipeline_enqueue_failed"
				continue
			}
			if _, ok := config.Cfg.CommentSites[site.SiteKey]; !ok {
				warnings[key] = "pipeline_enqueue_failed"
				continue
			}

			runID, err := ct.DB.CreateRun(siteID, "")
			if err != nil {
				warnings[key] = "pipeline_enqueue_failed"
				continue
			}
			if err := ct.Enqueuer.EnqueueRun(runID, site.SiteKey, ""); err != nil {
				_ = ct.DB.MarkRunFailed(runID, "enqueue", err.Error())
				warnings[key] = "pipeline_enqueue_failed"
				continue
			}
			batchRunIDs[key] = runID
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"results":       results,
		"count":         len(results),
		"batch_run_ids": batchRunIDs,
		"warnings":      warnings,
	})
}
