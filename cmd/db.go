package cmd

import (
	"context"
	"fmt"
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
	if err := database.SyncSitesFromConfig(ctx, config.Cfg.CommentSites); err != nil {
		_ = database.Close()
		return nil, nil, fmt.Errorf("sync sites from config failed: %w", err)
	}

	cleanup := func() { _ = database.Close() }
	return database, cleanup, nil
}
