package generator

import (
	"fmt"
	"strings"
	"time"

	"github.com/geschke/fyndmark/pkg/sanitize"
)

// ModerationMailInput contains all data required to build the moderation mail text.
type ModerationMailInput struct {
	SiteID     string
	PostPath   string
	EntryID    string
	ParentID   string
	CommentID  string
	Author     string
	AuthorUrl  string
	Email      string
	ClientIP   string
	Body       string
	CreatedAt  time.Time
	ApproveURL string
	RejectURL  string
}

// BuildModerationMail returns (subject, body, report) for the admin moderation email.
// It includes ONLY the sanitized comment body (full text), never the raw input body.
func BuildModerationMail(in ModerationMailInput) (string, string, sanitize.CommentBodyReport) {
	subject := fmt.Sprintf("[Fyndmark] New comment pending (%s)", in.SiteID)

	sanitized, report := sanitize.SanitizeCommentBodyWithReport(in.Body)

	var sb strings.Builder

	sb.WriteString("New comment pending\n\n")
	sb.WriteString("Site: " + in.SiteID + "\n")
	sb.WriteString("Comment ID: " + in.CommentID + "\n")

	if !in.CreatedAt.IsZero() {
		sb.WriteString("Created at: " + in.CreatedAt.Format(time.RFC3339) + "\n")
	}

	sb.WriteString("Post path: " + in.PostPath + "\n")
	if strings.TrimSpace(in.EntryID) != "" {
		sb.WriteString("Entry ID: " + strings.TrimSpace(in.EntryID) + "\n")
	}
	if strings.TrimSpace(in.ParentID) != "" {
		sb.WriteString("Parent ID: " + strings.TrimSpace(in.ParentID) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString("Author: " + in.Author + "\n")
	sb.WriteString("Email: " + in.Email + "\n\n")
	sb.WriteString("Client IP: " + strings.TrimSpace(in.ClientIP) + "\n\n")
	sb.WriteString("URL: " + in.AuthorUrl + "\n\n")

	sb.WriteString("Body (sanitized):\n")
	sb.WriteString(sanitized)
	if !strings.HasSuffix(sanitized, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Report section (short, factual)
	sb.WriteString("Notes:\n")
	sb.WriteString(fmt.Sprintf("- Sanitized changed output: %t\n", report.Changed))
	if report.DroppedFrontmatterBreaks > 0 {
		sb.WriteString(fmt.Sprintf("- Dropped standalone '---' lines: %d\n", report.DroppedFrontmatterBreaks))
	}
	if report.InvalidUTF8Fixed {
		sb.WriteString("- Fixed invalid UTF-8 sequences\n")
	}
	if report.RemovedNULBytes {
		sb.WriteString("- Removed NUL bytes\n")
	}
	if report.HTMLTagTokens > 0 || report.HTMLCommentTokens > 0 || report.HTMLDoctypeTokens > 0 {
		sb.WriteString(fmt.Sprintf("- HTML tokens removed: tags=%d, comments=%d, doctypes=%d\n",
			report.HTMLTagTokens, report.HTMLCommentTokens, report.HTMLDoctypeTokens))
	}
	if report.MarkdownLinks > 0 {
		sb.WriteString(fmt.Sprintf("- Markdown links degraded: %d\n", report.MarkdownLinks))
	}
	if report.MarkdownImages > 0 {
		sb.WriteString(fmt.Sprintf("- Markdown images degraded: %d\n", report.MarkdownImages))
	}
	sb.WriteString("\n")

	sb.WriteString("Approve:\n")
	sb.WriteString(in.ApproveURL)
	sb.WriteString("\n\n")

	sb.WriteString("Reject:\n")
	sb.WriteString(in.RejectURL)
	sb.WriteString("\n")

	return subject, sb.String(), report
}
