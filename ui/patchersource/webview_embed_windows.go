//go:build windows

package main

// On Windows we don't ship a webkit2 embed — instead we render a static
// placeholder tile and open URLs in the user's default browser. The dependency
// chain for an actual in-window webview on Windows is heavy (webview2 runtime,
// dotnet bridges) and the launcher uses webview only as a convenience preview.

import (
	"image"
	"image/color"
	"image/draw"
	"net/url"
	"os/exec"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

func StartModuleWebView(id, rawURL string, width, height int) *canvas.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{R: 30, G: 30, B: 45, A: 255}}, image.Point{}, draw.Src)
	c := canvas.NewImageFromImage(img)
	c.FillMode = canvas.ImageFillStretch
	c.SetMinSize(fyne.NewSize(float32(width), float32(height)))
	return c
}

func OpenEmbeddedWebView(rawURL string, width, height int) {
	if u, err := url.Parse(rawURL); err == nil && u.Scheme != "" {
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL).Start()
	}
}
