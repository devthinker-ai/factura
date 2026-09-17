package generate

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"strings"
)

const (
	maxLogoBytes = 512_000
	maxLogoDim   = 1600
	jpegQuality  = 85
)

// Template is PDF-layer branding. Nil / zero Template = unbranded default layout.
// Never reaches CII conversion — presentation only.
type Template struct {
	Logo       []byte // raw JPEG bytes
	HasLogo    bool
	Accent     [3]uint8
	HasAccent  bool
	HeaderText string
	FooterText string
}

// NormalizeLogo accepts PNG or JPEG bytes and returns JPEG q85 (alpha flattened on white).
func NormalizeLogo(raw []byte, mime string) ([]byte, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("generate: empty logo")
	}
	if len(raw) > maxLogoBytes {
		return nil, fmt.Errorf("generate: logo exceeds %d bytes", maxLogoBytes)
	}
	mime = strings.ToLower(strings.TrimSpace(mime))
	var img image.Image
	var err error
	switch mime {
	case "image/jpeg", "image/jpg":
		img, err = jpeg.Decode(bytes.NewReader(raw))
	case "image/png":
		img, err = png.Decode(bytes.NewReader(raw))
	default:
		return nil, fmt.Errorf("generate: unsupported logo mime %q", mime)
	}
	if err != nil {
		return nil, fmt.Errorf("generate: decode logo: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > maxLogoDim || h > maxLogoDim {
		return nil, fmt.Errorf("generate: logo dimensions %dx%d exceed %d", w, h, maxLogoDim)
	}
	// Flatten alpha onto white.
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Over)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, rgba, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("generate: encode jpeg: %w", err)
	}
	if out.Len() > maxLogoBytes {
		return nil, fmt.Errorf("generate: normalized logo exceeds %d bytes", maxLogoBytes)
	}
	return out.Bytes(), nil
}

// ParseAccent parses "#RRGGBB" into RGB. Empty → ok=false (no accent).
func ParseAccent(s string) (rgb [3]uint8, ok bool, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return rgb, false, nil
	}
	if len(s) != 7 || s[0] != '#' {
		return rgb, false, fmt.Errorf("generate: accent must be #RRGGBB")
	}
	for i := 1; i < 7; i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return rgb, false, fmt.Errorf("generate: accent must be #RRGGBB")
		}
	}
	r, g, b := hexByte(s[1:3]), hexByte(s[3:5]), hexByte(s[5:7])
	return [3]uint8{r, g, b}, true, nil
}

func hexByte(s string) uint8 {
	var v uint8
	for i := 0; i < 2; i++ {
		c := s[i]
		var d uint8
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		}
		v = v<<4 | d
	}
	return v
}

// LogoConfig returns JPEG width/height without full decode cost beyond DecodeConfig.
func LogoConfig(jpegBytes []byte) (w, h int, err error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpegBytes))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
