package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/db"
)

func openDatabase() (*db.DB, func(), error) {
	database, err := db.Open(config.Cfg.SQLite.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("db open failed (sqlite.path=%q): %w", config.Cfg.SQLite.Path, err)
	}

	if err := database.Migrate(); err != nil {
		_ = database.Close()
		return nil, nil, fmt.Errorf("db migrate failed: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	configuredSites, err := collectConfiguredSites(config.Cfg.CommentSites)
	if err != nil {
		_ = database.Close()
		return nil, nil, fmt.Errorf("collect configured site keys failed: %w", err)
	}

	if err := database.SyncSites(ctx, configuredSites); err != nil {
		_ = database.Close()
		return nil, nil, fmt.Errorf("sync sites from config failed: %w", err)
	}

	cleanup := func() { _ = database.Close() }
	return database, cleanup, nil
}

func collectConfiguredSites(cfg map[string]config.CommentsSiteConfig) (map[string]string, error) {
	out := make(map[string]string, len(cfg))

	for rawKey, siteCfg := range cfg {
		siteKey := strings.TrimSpace(rawKey)
		if siteKey == "" {
			return nil, fmt.Errorf("comment_sites contains empty key")
		}
		if _, exists := out[siteKey]; exists {
			return nil, fmt.Errorf("duplicate comment_sites key after trim: %q", siteKey)
		}

		title := strings.TrimSpace(siteCfg.Title)
		if title == "" { // fallback for empty titles
			title = siteKey
		}
		out[siteKey] = title

	}

	return out, nil
}
