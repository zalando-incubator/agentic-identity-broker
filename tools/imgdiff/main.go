// Command imgdiff compares two PNG images pixel-by-pixel and reports the
// percentage of differing pixels.  It is used by the screenshot CI workflow
// to avoid committing screenshots whose only changes are sub-pixel
// anti-aliasing or font-hinting noise.
//
// Usage:
//
//	imgdiff <file1.png> <file2.png> <threshold%>
//
// The threshold is a floating-point percentage (e.g. 0.5 means 0.5 %).
// Exit codes:
//
//	0  — difference is at or below the threshold (images are "the same")
//	1  — difference exceeds the threshold (images are meaningfully different)
//	2  — usage / I/O error
//
// On successful comparisons, the diff percentage is printed to stdout.
package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintf(os.Stderr, "Usage: imgdiff <image1.png> <image2.png> <threshold%%>\n")
		os.Exit(2)
	}

	threshold, err := strconv.ParseFloat(os.Args[3], 64)
	if err != nil || math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 100 {
		fmt.Fprintf(os.Stderr, "invalid threshold %q: must be a finite number between 0 and 100\n", os.Args[3])
		os.Exit(2)
	}

	img1, err := loadPNG(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading %s: %v\n", os.Args[1], err)
		os.Exit(2)
	}

	img2, err := loadPNG(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading %s: %v\n", os.Args[2], err)
		os.Exit(2)
	}

	pct := diffPercent(img1, img2)
	fmt.Printf("%.4f\n", pct)

	if pct > threshold {
		os.Exit(1)
	}
}

// toNRGBA converts any image.Image to *image.NRGBA for direct pixel-buffer
// comparison.  If the source is already NRGBA it is returned as-is.
func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok {
		return n
	}
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst
}

// diffPercent returns the percentage of pixels that differ between two images.
// If the images have different dimensions, 100 % is returned.
//
// Both images are first converted to NRGBA so their raw Pix slices can be
// compared directly — this avoids the per-pixel overhead of color-model
// conversions through the image.Image interface.
func diffPercent(a, b image.Image) float64 {
	ba := a.Bounds()
	bb := b.Bounds()

	if ba.Dx() != bb.Dx() || ba.Dy() != bb.Dy() {
		return 100.0
	}

	total := ba.Dx() * ba.Dy()
	if total == 0 {
		return 0.0
	}

	na := toNRGBA(a)
	nb := toNRGBA(b)

	diff := 0
	stride := na.Stride
	for y := 0; y < ba.Dy(); y++ {
		rowOff := y * stride
		for x := 0; x < ba.Dx(); x++ {
			off := rowOff + x*4
			if na.Pix[off] != nb.Pix[off] ||
				na.Pix[off+1] != nb.Pix[off+1] ||
				na.Pix[off+2] != nb.Pix[off+2] ||
				na.Pix[off+3] != nb.Pix[off+3] {
				diff++
			}
		}
	}

	return float64(diff) / float64(total) * 100.0
}

func loadPNG(path string) (img image.Image, err error) {
	f, err := os.Open(path) // #nosec G304,G703 -- command accepts user-selected image paths and has no privileged file root.
	if err != nil {
		return nil, err
	}
	defer func() {
		if cErr := f.Close(); cErr != nil && err == nil {
			err = cErr
		}
	}()
	return png.Decode(f)
}
