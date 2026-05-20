//go:build linux

package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

var (
	moduleImageCache = map[string]*canvas.Image{}
	moduleImageMu    sync.Mutex
)

func createPlaceholderPNG(path string, width, height int) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bgColor := color.RGBA{R: 30, G: 30, B: 45, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	png.Encode(f, img)
}

func StartModuleWebView(id, rawURL string, width, height int) *canvas.Image {
	if width <= 0 {
		width = 300
	}
	if height <= 0 {
		height = 200
	}

	outPath := fmt.Sprintf("%s/patcher_module_%s.png", os.TempDir(), id)
	createPlaceholderPNG(outPath, width, height)

	img := canvas.NewImageFromFile(outPath)
	img.FillMode = canvas.ImageFillStretch
	img.Resize(fyne.NewSize(float32(width), float32(height)))

	moduleImageMu.Lock()
	moduleImageCache[id] = img
	moduleImageMu.Unlock()

	go runWebViewHelper(id, rawURL, outPath, width, height)

	return img
}

func runWebViewHelper(id, rawURL, outPath string, width, height int) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("[WebView] cannot find executable:", err)
		return
	}
	helperPath := strings.TrimSuffix(exe, "/patcher") + "/webview_helper/webview_helper"

	if _, err := os.Stat(helperPath); err != nil {
		fmt.Println("[WebView] helper not found at:", helperPath)
		fmt.Println("[WebView] build it with: go build -tags webkit2_4_1 -o webview_helper/webview_helper webview_helper/main.go")
		return
	}

	cmd := exec.Command(helperPath,
		rawURL,
		outPath,
		fmt.Sprintf("%d", width),
		fmt.Sprintf("%d", height),
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("[WebView] stdout pipe error:", err)
		return
	}

	if err := cmd.Start(); err != nil {
		fmt.Println("[WebView] start error:", err)
		return
	}

	fmt.Printf("[WebView] helper started for %s\n", rawURL)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "READY:") {
			path := strings.TrimPrefix(line, "READY:")

			moduleImageMu.Lock()
			img, ok := moduleImageCache[id]
			moduleImageMu.Unlock()

			if ok && img != nil {
				img.File = path
				img.Refresh()
				canvas.Refresh(img)
				fmt.Printf("[WebView] updated image for %s\n", id)
			}
		}
	}
}

func OpenEmbeddedWebView(rawURL string, width, height int) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	u, err := url.Parse(rawURL)
	if err == nil {
		fyne.CurrentApp().OpenURL(u)
	}
}