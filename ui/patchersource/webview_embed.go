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
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

var (
	moduleImageCache = map[string]*canvas.Image{}
	moduleImageMu    sync.Mutex
)

func createPlaceholderPNG(path string, width, height int) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bg := color.RGBA{R: 30, G: 30, B: 45, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
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

func waylandEnv() []string {
	env := os.Environ()

	extra := []string{
		"GDK_BACKEND=x11",
		"WEBKIT_FORCE_SANDBOX=0",
		"DISPLAY=" + getEnv("DISPLAY", ":0"),
	}

	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		extra = append(extra, "XDG_RUNTIME_DIR="+v)
	}
	if v := os.Getenv("WAYLAND_DISPLAY"); v != "" {
		extra = append(extra, "WAYLAND_DISPLAY="+v)
	}

	overrides := map[string]string{}
	for _, kv := range extra {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			overrides[parts[0]] = kv
		}
	}
	out := make([]string, 0, len(env)+len(extra))
	for _, kv := range env {
		key := strings.SplitN(kv, "=", 2)[0]
		if replacement, ok := overrides[key]; ok {
			out = append(out, replacement)
			delete(overrides, key)
		} else {
			out = append(out, kv)
		}
	}
	for _, kv := range overrides {
		out = append(out, kv)
	}
	return out
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runWebViewHelper(id, rawURL, outPath string, width, height int) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("[WebView] cannot find own executable:", err)
		return
	}
	helperPath := strings.TrimSuffix(exe, "/patcher") + "/webview_helper/webview_helper"

	if _, err := os.Stat(helperPath); err != nil {
		fmt.Printf("[WebView] helper not found at %s\n", helperPath)
		fmt.Println("[WebView] build with: go build -tags webkit2_4_1 -o webview_helper/webview_helper webview_helper/main.go")
		writeLabelPNG(outPath, width, height, "webview_helper\nnot found")
		refreshModuleImage(id, outPath)
		return
	}

	cmd := exec.Command(helperPath,
		rawURL,
		outPath,
		fmt.Sprintf("%d", width),
		fmt.Sprintf("%d", height),
	)
	cmd.Env = waylandEnv()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("[WebView] stdout pipe error:", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("[WebView] stderr pipe error:", err)
		return
	}

	if err := cmd.Start(); err != nil {
		fmt.Println("[WebView] failed to start helper:", err)
		writeLabelPNG(outPath, width, height, "helper start\nfailed")
		refreshModuleImage(id, outPath)
		return
	}
	fmt.Printf("[WebView] helper started (pid %d) for %s\n", cmd.Process.Pid, rawURL)

	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			fmt.Println("[WebView/helper]", s.Text())
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "READY:") {
			path := strings.TrimPrefix(line, "READY:")
			refreshModuleImage(id, path)
			fmt.Printf("[WebView] frame updated for %s\n", id)
		}
	}

	if err := cmd.Wait(); err != nil {
		fmt.Println("[WebView] helper exited:", err)
	}
}

func refreshModuleImage(id, path string) {
	moduleImageMu.Lock()
	img, ok := moduleImageCache[id]
	moduleImageMu.Unlock()

	if !ok || img == nil {
		return
	}
	time.Sleep(20 * time.Millisecond)
	img.File = path
	img.Refresh()
	canvas.Refresh(img)
}

func writeLabelPNG(path string, width, height int, _ string) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bg := color.RGBA{R: 40, G: 30, B: 50, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	border := color.RGBA{R: 100, G: 80, B: 140, A: 255}
	for x := 0; x < width; x++ {
		img.Set(x, 0, border)
		img.Set(x, height-1, border)
	}
	for y := 0; y < height; y++ {
		img.Set(0, y, border)
		img.Set(width-1, y, border)
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	png.Encode(f, img)
}

func OpenEmbeddedWebView(rawURL string, width, height int) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	sizeFlag := fmt.Sprintf("--window-size=%d,%d", width, height)

	candidates := [][]string{
		{"chromium", "--app=" + rawURL, sizeFlag, "--no-first-run", "--no-default-browser-check"},
		{"chromium-browser", "--app=" + rawURL, sizeFlag, "--no-first-run", "--no-default-browser-check"},
		{"google-chrome", "--app=" + rawURL, sizeFlag, "--no-first-run", "--no-default-browser-check"},
		{"google-chrome-stable", "--app=" + rawURL, sizeFlag, "--no-first-run"},
		{"firefox", "--new-window", rawURL},
		{"firefox-esr", "--new-window", rawURL},
	}

	for _, args := range candidates {
		binPath, err := exec.LookPath(args[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(binPath, args[1:]...)
		cmd.Env = os.Environ()
		if err := cmd.Start(); err == nil {
			fmt.Printf("[WebView] opened %s with %s\n", rawURL, args[0])
			go cmd.Wait()
			return
		}
	}

	xdg, err := exec.LookPath("xdg-open")
	if err == nil {
		cmd := exec.Command(xdg, rawURL)
		cmd.Env = os.Environ()
		cmd.Start()
		return
	}

	u, err := url.Parse(rawURL)
	if err == nil {
		fyne.CurrentApp().OpenURL(u)
	}
}