package mailer

import (
	"fmt"
	"strings"

	"github.com/geschke/fyndmark/config"
	mail "github.com/wneessen/go-mail"
)

// SendTextMail sends a plain text email using the global SMTP config.
func SendTextMail(recipients []string, subject, body string) error {
	smtpCfg := config.Cfg.SMTP

	var opts []mail.Option

	if smtpCfg.Port > 0 {
		opts = append(opts, mail.WithPort(smtpCfg.Port))
	}

	switch strings.ToLower(strings.TrimSpace(smtpCfg.TLSPolicy)) {
	case "none":
		opts = append(opts, mail.WithTLSPortPolicy(mail.NoTLS))
	case "opportunistic":
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSOpportunistic))
	case "", "mandatory":
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSMandatory))
	default:
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSMandatory))
	}

	client, err := mail.NewClient(smtpCfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("failed to create mail client: %w", err)
	}

	if smtpCfg.Username != "" && smtpCfg.Password != "" {
		client.SetSMTPAuth(mail.SMTPAuthPlain)
		client.SetUsername(smtpCfg.Username)
		client.SetPassword(smtpCfg.Password)
	}

	msg := mail.NewMsg()
	if err := msg.From(smtpCfg.From); err != nil {
		return fmt.Errorf("invalid FROM address: %w", err)
	}

	if len(recipients) == 0 {
		return fmt.Errorf("no recipients configured")
	}
	for _, rcpt := range recipients {
		if err := msg.To(rcpt); err != nil {
			return fmt.Errorf("invalid recipient %q: %w", rcpt, err)
		}
	}

	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, body)

	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("failed to send mail: %w", err)
	}

	return nil
}
