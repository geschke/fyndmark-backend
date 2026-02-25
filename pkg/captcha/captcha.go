package captcha

import (
	"fmt"
	"strings"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/captcha/hcaptcha"
	"github.com/geschke/fyndmark/pkg/captcha/turnstile"
)

type Provider interface {
	Validate(token, remoteIP string) (bool, []string, error)
}

// ResolveProvider performs its package-specific operation.
func ResolveProvider(cfg *config.CaptchaConfig) (Provider, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	name := strings.TrimSpace(strings.ToLower(cfg.Provider))
	switch name {
	case "turnstile":
		return turnstile.New(cfg.SecretKey)
	case "hcaptcha":
		return hcaptcha.New(cfg.SecretKey)
	default:
		return nil, fmt.Errorf("unknown captcha provider %q", cfg.Provider)
	}
}
