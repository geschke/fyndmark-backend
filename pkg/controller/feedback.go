package controller

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/geschke/fyntral/config"
	"github.com/geschke/fyntral/pkg/cors"
	"github.com/geschke/fyntral/pkg/turnstile"
	"github.com/gin-gonic/gin"
	mail "github.com/wneessen/go-mail"
)

// FeedbackController
type FeedbackController struct {
}

func NewFeedbackController() *FeedbackController {
	ct := FeedbackController{}
	return &ct
}

// POST /form/:formid
func (ct FeedbackController) PostMail(c *gin.Context) {
	formID := c.Param("formid")
	log.Println("PostMail called for form:", formID)

	// Look up form configuration by ID
	formCfg, ok := config.Cfg.Forms[formID]
	if !ok {
		log.Printf("Unknown form ID: %s", formID)
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "unknown_form",
		})
		return
	}

	// Apply CORS based on the form's allowed origins.
	// If this returns false, the response is already handled (403 or 204).
	if !cors.ApplyCORS(c, formCfg.CORSAllowedOrigins) {
		return
	}

	// Turnstile verification (per form config)
	token := c.PostForm("cf-turnstile-response")
	tsCfg := formCfg.Turnstile

	okTS, tsErrors, err := turnstile.Validate(token, c.ClientIP(), tsCfg.SecretKey, tsCfg.Enabled)
	if err != nil {
		log.Printf("Turnstile verification error for form %s: %v", formID, err)
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

	// From here on, CORS is OK and this is not a preflight request.
	// Collect and validate form values:
	values, fieldErrors, err := collectAndValidateFormValues(c, formCfg)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":      false,
			"error":        "validation_failed",
			"field_errors": fieldErrors,
		})
		return
	}

	subject, body := buildMailContent(formID, formCfg, values)

	if err := sendFormMail(formCfg, subject, body); err != nil {
		log.Printf("Error sending mail for form %s: %v", formID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "mail_send_failed",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"formId":  formID,
	})
}

// collectAndValidateFormValues reads all configured fields from the request,
// validates required fields and returns a map of field name to submitted value.
func collectAndValidateFormValues(c *gin.Context, formCfg config.FormConfig) (map[string]string, map[string]string, error) {
	values := make(map[string]string)
	fieldErrors := make(map[string]string)

	for _, field := range formCfg.Fields {
		value := strings.TrimSpace(c.PostForm(field.Name))

		if field.Required && value == "" {
			fieldErrors[field.Name] = "missing_required_field"
		}

		// Very simple email sanity check (optional, can be improved later)
		if field.Type == "email" && value != "" && !strings.Contains(value, "@") {
			// Do not overwrite an existing error for this field (if any)
			if _, exists := fieldErrors[field.Name]; !exists {
				fieldErrors[field.Name] = "invalid_email"
			}
		}

		values[field.Name] = value
	}
	if len(fieldErrors) > 0 {
		return values, fieldErrors, fmt.Errorf("form has validation errors")
	}
	return values, fieldErrors, nil
}

// buildMailContent builds the mail subject and body from form config and values.
func buildMailContent(formID string, formCfg config.FormConfig, values map[string]string) (string, string) {
	// Subject
	subject := formCfg.SubjectPrefix
	if subject == "" {
		subject = "[Fyntral feedback]"
	}
	if formCfg.Title != "" {
		subject = subject + " " + formCfg.Title
	}

	// Plain-text body
	var sb strings.Builder
	sb.WriteString("Form ID: " + formID + "\n")
	if formCfg.Title != "" {
		sb.WriteString("Form title: " + formCfg.Title + "\n")
	}
	sb.WriteString("\nSubmitted values:\n\n")

	for _, f := range formCfg.Fields {
		sb.WriteString(f.Label)
		sb.WriteString(" (")
		sb.WriteString(f.Name)
		sb.WriteString("): ")
		sb.WriteString(values[f.Name])
		sb.WriteString("\n")
	}

	body := sb.String()
	return subject, body
}

// sendFormMail sends the mail using the global SMTP config and the given form config.
func sendFormMail(formCfg config.FormConfig, subject, body string) error {
	smtpCfg := config.Cfg.SMTP

	var opts []mail.Option

	// Optional explicit port
	if smtpCfg.Port > 0 {
		opts = append(opts, mail.WithPort(smtpCfg.Port))
	}

	// TLS policy
	switch strings.ToLower(strings.TrimSpace(smtpCfg.TLSPolicy)) {
	case "none":
		// Explicitly disable TLS / STARTTLS
		opts = append(opts, mail.WithTLSPortPolicy(mail.NoTLS))
	case "opportunistic":
		// Try TLS (STARTTLS) if supported, else fall back to plain SMTP
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSOpportunistic))
	case "", "mandatory":
		// Default: TLS required (STARTTLS). Fail if server does not support TLS.
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSMandatory))
	default:
		// Unknown value → be conservative and require TLS
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSMandatory))
	}

	// Create client
	client, err := mail.NewClient(smtpCfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("failed to create mail client: %w", err)
	}

	// Optional authentication
	if smtpCfg.Username != "" && smtpCfg.Password != "" {
		client.SetSMTPAuth(mail.SMTPAuthPlain)
		client.SetUsername(smtpCfg.Username)
		client.SetPassword(smtpCfg.Password)
	}

	// Build message
	msg := mail.NewMsg()
	if err := msg.From(smtpCfg.From); err != nil {
		return fmt.Errorf("invalid FROM address: %w", err)
	}

	if len(formCfg.Recipients) == 0 {
		return fmt.Errorf("no recipients configured for this form")
	}
	for _, rcpt := range formCfg.Recipients {
		if err := msg.To(rcpt); err != nil {
			return fmt.Errorf("invalid recipient %q: %w", rcpt, err)
		}
	}

	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, body)

	// Send mail
	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("failed to send mail: %w", err)
	}

	return nil
}
