/*
Package config provides utilities for initializing, loading,
and validating configuration parameters required by the application.
It uses Viper for reading configuration files and setting global variables.
*/

package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	//	"github.com/geschke/fyndmark/pkg/dbconn"
	//	logging "github.com/geschke/goar/pkg/logging"
	"github.com/spf13/viper"
)

// ServerConfig holds settings related to the HTTP server.
type ServerConfig struct {
	// Listen is the address the HTTP server should bind to, e.g. ":8080" or "0.0.0.0:8080".
	Listen string `mapstructure:"listen"`
}

// SQLiteConfig holds settings for the SQLite database file.
type SQLiteConfig struct {
	Path string `mapstructure:"path"`
}

// HugoConfig controls whether Hugo should be executed by fyndmark (optional).
type HugoConfig struct {
	// Disables controls whether the backend should run Hugo after generating markdown files, default false, so Hugo will run. Set to true if this step should be skipped.
	Disabled bool `mapstructure:"disabled"`
}

// CommentsSiteConfig describes one logical site/blog for comments.
type CommentsSiteConfig struct {
	Title              string         `mapstructure:"title"`
	CORSAllowedOrigins []string       `mapstructure:"cors_allowed_origins"`
	Captcha            *CaptchaConfig `mapstructure:"captcha"`

	AdminRecipients []string   `mapstructure:"admin_recipients"`
	TokenSecret     string     `mapstructure:"token_secret"`
	Git             GitConfig  `mapstructure:"git"`
	Hugo            HugoConfig `mapstructure:"hugo"`
	Timezone        string     `mapstructure:"timezone"`
}

type GitConfig struct {
	RepoURL     string `mapstructure:"repo_url"`
	Branch      string `mapstructure:"branch"`
	AccessToken string `mapstructure:"access_token"`
	CloneDir    string `mapstructure:"clone_dir"`
	Depth       int    `mapstructure:"depth"`

	// Optional: initialize/update submodules during clone
	RecurseSubmodules bool `mapstructure:"recurse_submodules"`

	// Optional: additional themes/components to ensure exist under the cloned repo
	Themes []GitThemeConfig `mapstructure:"themes"`
}

// GitThemeConfig describes an additional theme/component repository that should be
// cloned into the website working copy (typically under themes/).
type GitThemeConfig struct {
	// Logical name (only for readability/logging)
	Name string `mapstructure:"name"`

	// Repo URL of the theme/component (https://... or git@...).
	RepoURL string `mapstructure:"repo_url"`

	// Optional branch (if empty, default branch is used)
	Branch string `mapstructure:"branch"`

	// Target path within the cloned website repo, e.g. "themes/hugo-felmdrav"
	TargetPath string `mapstructure:"target_path"`

	// Optional token for private theme repos (leave empty for public repos)
	AccessToken string `mapstructure:"access_token"`

	// Optional shallow clone depth for this theme repo (0 = full clone)
	Depth int `mapstructure:"depth"`
}

// SMTPConfig holds settings related to the sending mail server
type SMTPConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`     // 0 = library default
	Username string `mapstructure:"username"` // optional
	Password string `mapstructure:"password"` // optional
	From     string `mapstructure:"from"`

	// TLSMode / policy for SMTP:
	//   "none"          → no TLS (plain SMTP, e.g. local server on port 25)
	//   "opportunistic" → use TLS if possible, else fall back to plain
	//   "mandatory"     → require TLS/STARTTLS, fail if not supported
	TLSPolicy string `mapstructure:"tls_policy"`
}

// FieldConfig describes a single form field.
type FieldConfig struct {
	Name     string   `mapstructure:"name"`
	Label    string   `mapstructure:"label"`
	Type     string   `mapstructure:"type"`
	Required bool     `mapstructure:"required"`
	Options  []string `mapstructure:"options"`
}

type CaptchaConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Provider  string `mapstructure:"provider"`
	SecretKey string `mapstructure:"secret_key"`
	SiteKey   string `mapstructure:"site_key"`
}

// FormConfig describes one logical form (e.g. feedback form for a specific site).
type FormConfig struct {
	Title              string         `mapstructure:"title"`
	Recipients         []string       `mapstructure:"recipients"`
	SubjectPrefix      string         `mapstructure:"subject_prefix"`
	CORSAllowedOrigins []string       `mapstructure:"cors_allowed_origins"`
	Fields             []FieldConfig  `mapstructure:"fields"`
	Captcha            *CaptchaConfig `mapstructure:"captcha"`
}

// AppConfig is the main configuration struct for the entire application.
type AppConfig struct {
	Server ServerConfig `mapstructure:"server"`
	SMTP   SMTPConfig   `mapstructure:"smtp"`
	//CORS   CORSConfig            `mapstructure:"cors"` // maybe later
	Forms map[string]FormConfig `mapstructure:"forms"`

	SQLite       SQLiteConfig                  `mapstructure:"sqlite"`
	CommentSites map[string]CommentsSiteConfig `mapstructure:"comment_sites"`

	// Logging config kept for future extensions, currently unused.
	// LogLevel  string `mapstructure:"log_level"`
	// LogFile   string `mapstructure:"log_file"`
	// LogFormat string `mapstructure:"log_format"`
}

// Global configuration variables
var (
	//DocDbConfig dbconn.DocumentDatabaseConfiguration

	//LogLevel  string
	//LogFile   string
	//LogFormat string

	//Host string
	//Port int
	Cfg AppConfig
)

// Global configuration constants

// setLogging initializes the global logging system using configuration values
// provided by Viper. It reads the log file path and log level, configures the
// logger accordingly, and writes an informational startup message. The function
// returns an error if logger initialization fails.
/*func setLogging() error {
	cfg := logging.Config{
		LogFile:   viper.GetString("log_file"),
		LogLevel:  viper.GetString("log_level"),
		LogFormat: viper.GetString("log_format"),
	}
	if err := logging.Init(cfg); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}
	logging.Infof("goar backend started, log destination: %s", cfg.LogFile)
	return nil
}*/

// InitAndLoad is the single entrypoint to initialize and load configuration.
// It prepares Viper, reads the config (with .env fallback), unmarshals into Cfg
// and performs basic validation.
func InitAndLoad(cfgFile string) error {
	setupViper(cfgFile)

	if err := readAndSetConfig(); err != nil {
		return err
	}

	return nil
}

// setupViper configures Viper's search paths and environment mapping,
// but does NOT read or unmarshal the config yet.
func setupViper(cfgFile string) {
	if cfgFile != "" {
		// Config file explicitly provided via CLI (--config).
		// Viper will detect the file type automatically based on extension.
		viper.SetConfigFile(cfgFile)
	} else {
		// No config provided → search for config.* in common folders.
		viper.AddConfigPath(".")
		viper.AddConfigPath("./config")
		viper.AddConfigPath("/config")

		// Try config.yaml / config.yml / config.json / config.toml first.
		viper.SetConfigName("config")
	}

	// Map nested keys like "cors.allowed_origins" to env vars "CORS_ALLOWED_ORIGINS".
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Allow environment variables to override config values.
	viper.AutomaticEnv()
}

// readAndSetConfig reads the configuration (with .env fallback),
// unmarshals it into the global Cfg struct and applies basic validation.
func readAndSetConfig() error {
	// Try to read the primary config file (config.* or whatever was set).
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Primary config not found (%v). Falling back to .env file...", err)

		// Fallback: try .env file explicitly.
		viper.SetConfigName(".env")
		viper.SetConfigType("env") // .env has no extension → must set manually.

		if err2 := viper.ReadInConfig(); err2 != nil {
			// Final fallback: use environment variables only.
			log.Printf("No .env file found either. Using environment variables only. (%v)", err2)
		}

	}

	// Unmarshal configuration into our AppConfig struct.
	if err := viper.Unmarshal(&Cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Basic validation for server listen address.
	if Cfg.Server.Listen == "" {
		return exitOnErr(errors.New("server.listen must be set in config or environment"))
	}

	log.Println("server.listen:", Cfg.Server.Listen)

	if Cfg.SQLite.Path == "" {
		return exitOnErr(errors.New("sqlite.path must be set in config or environment"))
	}
	log.Println("sqlite.path:", Cfg.SQLite.Path)

	for siteID, siteCfg := range Cfg.CommentSites {
		if len(siteCfg.AdminRecipients) == 0 {
			return exitOnErr(fmt.Errorf("comment_sites.%s.admin_recipients must be set", siteID))
		}
		if strings.TrimSpace(siteCfg.TokenSecret) == "" {
			return exitOnErr(fmt.Errorf("comment_sites.%s.token_secret must be set", siteID))
		}
		if siteCfg.Captcha != nil {
			if strings.TrimSpace(siteCfg.Captcha.Provider) == "" {
				return exitOnErr(fmt.Errorf("comment_sites.%s.captcha.provider must be set", siteID))
			}
			if strings.TrimSpace(siteCfg.Captcha.SecretKey) == "" {
				return exitOnErr(fmt.Errorf("comment_sites.%s.captcha.secret_key must be set", siteID))
			}
		}
	}

	for formID, formCfg := range Cfg.Forms {
		if len(formCfg.Recipients) == 0 {
			return exitOnErr(fmt.Errorf("forms.%s.recipients must be set", formID))
		}
		if formCfg.Captcha != nil {
			if strings.TrimSpace(formCfg.Captcha.Provider) == "" {
				return exitOnErr(fmt.Errorf("forms.%s.captcha.provider must be set", formID))
			}
			if strings.TrimSpace(formCfg.Captcha.SecretKey) == "" {
				return exitOnErr(fmt.Errorf("forms.%s.captcha.secret_key must be set", formID))
			}
		}
	}

	// maybe later enable logging config

	return nil
}

// exitOnErr prints an error to stderr and exits the process.
// It also returns the same error for completeness, even though it's never reached.
func exitOnErr(err error) error {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
	return err
}
