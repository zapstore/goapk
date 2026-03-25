// Package icon handles launcher icon processing: resizing a source PNG to all
// Android density buckets and generating monochrome variants.
package icon

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	xdraw "golang.org/x/image/draw"
)

// Sized holds a resized icon image and the density it was produced for.
type Sized struct {
	Name  string // density name e.g. "mdpi"
	PxSz  int    // pixel dimension (square)
	Image image.Image
}

// ResizeToAll resizes srcPath to all standard Android launcher icon sizes.
// Returns one Sized per density, ordered mdpi → xxxhdpi.
func ResizeToAll(srcPath string, sizes []int, names []string) ([]Sized, error) {
	src, err := loadPNG(srcPath)
	if err != nil {
		return nil, fmt.Errorf("loading icon %q: %w", srcPath, err)
	}
	out := make([]Sized, len(sizes))
	for i, sz := range sizes {
		resized := resizeImage(src, sz, sz)
		out[i] = Sized{Name: names[i], PxSz: sz, Image: resized}
	}
	return out, nil
}

// Monochrome converts a color image to a monochrome (grayscale→alpha mask) PNG.
// The output is an NRGBA image where non-transparent pixels are black and the
// original alpha channel is preserved. This matches Android's monochrome icon spec.
func Monochrome(img image.Image) image.Image {
	bounds := img.Bounds()
	dst := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			// luminance (approximate, 16-bit → 8-bit)
			lum := uint8((19595*r + 38470*g + 7471*b + 1<<15) >> 24)
			// Use lum as the alpha multiplier so the icon appears as a solid tinted shape.
			// The colour is black; Android's adaptive icon tinting handles the actual hue.
			dst.SetNRGBA(x, y, color.NRGBA{
				R: 0,
				G: 0,
				B: 0,
				A: uint8(uint32(lum) * uint32(a>>8) / 255),
			})
		}
	}
	return dst
}

// EncodePNG encodes an image to PNG bytes.
func EncodePNG(img image.Image) ([]byte, error) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	errCh := make(chan error, 1)
	var result []byte
	go func() {
		defer pr.Close()
		buf := make([]byte, 0, 64*1024)
		tmp := make([]byte, 4096)
		for {
			n, rerr := pr.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if rerr != nil {
				break
			}
		}
		result = buf
		errCh <- nil
	}()
	if err := png.Encode(pw, img); err != nil {
		pw.Close()
		return nil, fmt.Errorf("encoding PNG: %w", err)
	}
	pw.Close()
	<-errCh
	return result, nil
}

// loadPNG loads a PNG file and returns the decoded image.
func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}
	return img, nil
}

// resizeImage scales src to w×h using Catmull-Rom resampling.
func resizeImage(src image.Image, w, h int) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}
