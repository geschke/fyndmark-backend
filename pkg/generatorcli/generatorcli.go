package generatorcli

import (
	"context"

	"github.com/geschke/fyndmark/pkg/generator"
)

// Generate is a thin wrapper to keep the CLI-layer simple.
func Generate(ctx context.Context, siteID string) error {
	return generator.GenerateWithContext(ctx, siteID)
}

// GenerateBackground is a convenience wrapper using context.Background().
func GenerateBackground(siteID string) error {
	return generator.Generate(siteID)
}
