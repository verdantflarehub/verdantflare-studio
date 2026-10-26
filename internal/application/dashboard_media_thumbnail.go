package application

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	"github.com/HugoSmits86/nativewebp"
	_ "golang.org/x/image/webp"
)

// World evidence uses the same stable preview contract without requiring a
// Project file. Decode only on a derived-cache miss, with bounded work/memory.
var mediaDecodeSlots = make(chan struct{}, 2)
var mediaDecodeWaiting = make(chan struct{}, 8)

func encodeMediaThumbnail(ctx context.Context, source io.ReadSeeker, edge int, output io.Writer) error {
	select {
	case mediaDecodeWaiting <- struct{}{}:
	default:
		return mediaError{contentFailure(429, "")}
	}
	defer func() { <-mediaDecodeWaiting }()
	select {
	case mediaDecodeSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-mediaDecodeSlots }()
	raw, err := io.ReadAll(io.LimitReader(source, (32<<20)+1))
	if err != nil || len(raw) > 32<<20 {
		return mediaError{contentFailure(413, "")}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return mediaError{contentFailure(415, "")}
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return mediaError{contentFailure(413, "")}
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return mediaError{contentFailure(415, "")}
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	longest := max(w, h)
	if longest > edge {
		nw, nh := max(1, w*edge/longest), max(1, h*edge/longest)
		scaled := image.NewNRGBA(image.Rect(0, 0, nw, nh))
		for y := 0; y < nh; y++ {
			for x := 0; x < nw; x++ {
				scaled.Set(x, y, img.At(bounds.Min.X+x*w/nw, bounds.Min.Y+y*h/nh))
			}
		}
		img = scaled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nativewebp.Encode(output, img, nil)
}
