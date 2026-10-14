package sandbox

import (
	"context"

	"github.com/jailwick/jailwick/internal/image"
)

// CheckImage tries an image for jailwick before a start would fail on it (see image.Check).
func (m *Manager) CheckImage(ctx context.Context, o image.CheckOptions) ([]image.Finding, error) {
	return m.images().Check(ctx, o)
}
