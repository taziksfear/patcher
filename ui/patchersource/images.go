package main

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
)

// Theme art is full-resolution wallpaper — a 2380x1439 JPEG costs ~14MB once
// decoded to RGBA. The launcher rebuilds its whole canvas on every edit and
// draws every installed theme's art on the Themes page, so handing those files
// straight to canvas.NewImageFromFile meant decoding tens of megabytes over and
// over. Everything here decodes once, at the size actually drawn, and keeps the
// result until the file changes.

type cachedImage struct {
	mod  time.Time
	size int64
	img  image.Image
}

var (
	imageCacheMu sync.Mutex
	imageCache   = map[string]cachedImage{}
)

// thumbnail decodes path and scales it down to fit inside maxW x maxH, keeping
// its aspect ratio. Images already smaller than that are returned as-is.
func thumbnail(path string, maxW, maxH int) image.Image {
	return decodeScaled(path, maxW, maxH, false)
}

// stretched decodes path and scales it to exactly w x h, matching what
// canvas.ImageFillStretch would otherwise do at full resolution every frame.
func stretched(path string, w, h int) image.Image {
	return decodeScaled(path, w, h, true)
}

func decodeScaled(path string, w, h int, exact bool) image.Image {
	if w <= 0 || h <= 0 {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	key := fmt.Sprintf("%s|%dx%d|%t", path, w, h, exact)

	imageCacheMu.Lock()
	if c, ok := imageCache[key]; ok && c.mod.Equal(info.ModTime()) && c.size == info.Size() {
		imageCacheMu.Unlock()
		return c.img
	}
	imageCacheMu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		fmt.Printf("[images] decode %s: %v\n", path, err)
		return nil
	}

	out := src
	bounds := src.Bounds()
	dstW, dstH := w, h
	if !exact {
		dstW, dstH = fitWithin(bounds.Dx(), bounds.Dy(), w, h)
	}
	if dstW != bounds.Dx() || dstH != bounds.Dy() {
		dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
		xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, xdraw.Src, nil)
		out = dst
	}

	imageCacheMu.Lock()
	imageCache[key] = cachedImage{mod: info.ModTime(), size: info.Size(), img: out}
	imageCacheMu.Unlock()
	return out
}

// fitWithin scales srcW x srcH down to fit inside maxW x maxH. It never scales
// up — upscaling would cost memory for no visual gain.
func fitWithin(srcW, srcH, maxW, maxH int) (int, int) {
	if srcW <= maxW && srcH <= maxH {
		return srcW, srcH
	}
	scale := float64(maxW) / float64(srcW)
	if s := float64(maxH) / float64(srcH); s < scale {
		scale = s
	}
	w := int(float64(srcW) * scale)
	h := int(float64(srcH) * scale)
	return max(w, 1), max(h, 1)
}
