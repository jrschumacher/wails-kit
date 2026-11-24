package desktop

import (
	"fmt"
	"strconv"

	"github.com/jrschumacher/wails-kit/errors"
)

// Colour is an opaque sRGB colour, as a window's background takes it.
type Colour struct {
	R, G, B uint8
}

// Hex returns the colour as #rrggbb.
func (c Colour) Hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// ParseColour reads a #rrggbb colour, the form tokens.css and an app's
// theme file write --wk-color-bg in.
func ParseColour(s string) (Colour, error) {
	if len(s) != 7 || s[0] != '#' {
		return Colour{}, errors.Newf(ErrInvalidColour, "not a #rrggbb colour: %q", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return Colour{}, errors.Wrap(ErrInvalidColour, fmt.Sprintf("not a #rrggbb colour: %q", s), err)
	}
	return Colour{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v)}, nil
}

// MustParseColour is ParseColour for constants; it panics on a bad value.
func MustParseColour(s string) Colour {
	c, err := ParseColour(s)
	if err != nil {
		panic(err)
	}
	return c
}

// The kit's --wk-color-bg in frontend/ui/src/tokens.css, light and dark. A
// test keeps them in step with the stylesheet.
var (
	DefaultBackgroundLight = Colour{0xfc, 0xfc, 0xfb}
	DefaultBackgroundDark  = Colour{0x18, 0x1a, 0x1f}
)

// Appearance is the app's own choice of light or dark, as the frontend's
// <html data-theme> holds it. AppearanceSystem follows the system.
type Appearance string

// The appearances.
const (
	AppearanceSystem Appearance = ""
	AppearanceLight  Appearance = "light"
	AppearanceDark   Appearance = "dark"
)

// Background returns the window background for the current appearance:
// the theme's bg in dark when the app is set to dark, or when it follows
// the system and systemDark is true; otherwise its bg in light.
func (s *Service) Background(systemDark bool) Colour {
	s.mu.Lock()
	defer s.mu.Unlock()
	dark := s.appearance == AppearanceDark || (s.appearance == AppearanceSystem && systemDark)
	if dark {
		return s.bgDark
	}
	return s.bgLight
}

// SetAppearance records the app's own light or dark setting, so Background
// follows it instead of the system. Attach repaints every window.
func (s *Service) SetAppearance(a Appearance) {
	s.mu.Lock()
	s.appearance = a
	f := s.onAppearance
	s.mu.Unlock()
	if f != nil {
		f()
	}
}
