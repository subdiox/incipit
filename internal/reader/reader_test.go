package reader

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// makeCBZ writes a CBZ with image entries (deliberately out of natural order)
// plus a non-image file that must be ignored.
func makeCBZ(t *testing.T, w, h int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "comic.cbz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	entries := []string{"page10.png", "page2.png", "page1.png", "info.txt"}
	for _, name := range entries {
		w2, _ := zw.Create(name)
		if name == "info.txt" {
			w2.Write([]byte("not an image"))
			continue
		}
		w2.Write(makePNG(t, w, h))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPagesNaturalOrderAndFilter(t *testing.T) {
	cbz := makeCBZ(t, 50, 80)
	pages, err := Pages(cbz)
	if err != nil {
		t.Fatalf("Pages: %v", err)
	}
	want := []string{"page1.png", "page2.png", "page10.png"}
	if len(pages) != len(want) {
		t.Fatalf("pages = %v, want %v", pages, want)
	}
	for i := range want {
		if pages[i] != want[i] {
			t.Errorf("pages[%d] = %q, want %q (full: %v)", i, pages[i], want[i], pages)
		}
	}
}

func TestRenderPageOriginalAndResized(t *testing.T) {
	cbz := makeCBZ(t, 400, 600)
	pages, _ := Pages(cbz)
	svc := NewService(filepath.Join(t.TempDir(), "cache"))

	// Original page: raw PNG bytes, correct content type.
	orig, err := svc.RenderPage(cbz, pages, 0, 0, 12345)
	if err != nil {
		t.Fatalf("RenderPage original: %v", err)
	}
	if orig.ContentType != "image/png" {
		t.Errorf("original content type = %q", orig.ContentType)
	}
	if img, _, err := image.Decode(bytes.NewReader(orig.Data)); err != nil || img.Bounds().Dx() != 400 {
		t.Errorf("original decode: dx=%d err=%v", safeDx(img), err)
	}

	// Resized page: JPEG bounded to width 100, cached on disk.
	res, err := svc.RenderPage(cbz, pages, 0, 100, 12345)
	if err != nil {
		t.Fatalf("RenderPage resized: %v", err)
	}
	if res.ContentType != "image/jpeg" {
		t.Errorf("resized content type = %q", res.ContentType)
	}
	img, err := jpeg.Decode(bytes.NewReader(res.Data))
	if err != nil {
		t.Fatalf("decode resized jpeg: %v", err)
	}
	if img.Bounds().Dx() != 100 {
		t.Errorf("resized width = %d, want 100", img.Bounds().Dx())
	}

	// Second render hits the disk cache (same ETag/key) and matches.
	res2, _ := svc.RenderPage(cbz, pages, 0, 100, 12345)
	if res2.ETag != res.ETag || !bytes.Equal(res.Data, res2.Data) {
		t.Error("cached render differs from first render")
	}

	// Out-of-range page.
	if _, err := svc.RenderPage(cbz, pages, 99, 0, 1); err != ErrPageOutOfRange {
		t.Errorf("out of range => %v", err)
	}
}

func TestFirstPageJPEGForCover(t *testing.T) {
	cbz := makeCBZ(t, 300, 450)
	svc := NewService(filepath.Join(t.TempDir(), "cache"))
	data, err := svc.FirstPageJPEG(cbz, 200)
	if err != nil {
		t.Fatalf("FirstPageJPEG: %v", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode cover: %v", err)
	}
	if img.Bounds().Dx() != 200 {
		t.Errorf("cover width = %d, want 200", img.Bounds().Dx())
	}
}

func safeDx(img image.Image) int {
	if img == nil {
		return -1
	}
	return img.Bounds().Dx()
}
