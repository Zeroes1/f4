package terminal

import (
	"context"
	"errors"
	"image"

	"github.com/unxed/goclip"
	"github.com/unxed/vtui"
)

// ClipboardContents captures the representations offered for one paste.
type ClipboardContents struct {
	Image image.Image
	Text  string
}

func ReadClipboardContents(ctx context.Context) (ClipboardContents, error) {
	contents, err := goclip.ReadContents(ctx)
	img := contents.Image
	if err != nil && !errors.Is(err, goclip.ErrNoImage) && !errors.Is(err, goclip.ErrUnsupportedFormat) && !errors.Is(err, goclip.ErrUnavailable) {
		return ClipboardContents{}, err
	}
	if err := ctx.Err(); err != nil {
		return ClipboardContents{}, err
	}
	if img != nil {
		return ClipboardContents{Image: img, Text: contents.Text}, nil
	}

	// Keep vtui's far2l, authorization and terminal integrations for text.
	return ClipboardContents{Image: img, Text: vtui.GetClipboard()}, nil
}
