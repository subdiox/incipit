// Command seed populates a Calibre library with a few sample CBZ comics so a
// freshly-cloned Incipit has something to browse and read out of the box.
//
// It imports each book through the same internal/calibre.AddBook write path the
// server uses, so every invariant (folder layout, metadata.opf, cover, data
// rows, the title_sort/uuid4 triggers) is satisfied — nothing is hand-written
// into metadata.db.
//
// Usage:
//
//	INCIPIT_LIBRARY=./library go run ./cmd/seed        # seed only if empty
//	INCIPIT_LIBRARY=./library go run ./cmd/seed -force # add the samples anyway
//
// The generated pages are plain procedurally-drawn placeholders (a colored
// background with a big page number and the title) — no external assets, no
// copyrighted material.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"os"
	"time"

	"github.com/disintegration/imaging"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"incipit/internal/calibre"
)

// sampleBook describes one book to generate.
type sampleBook struct {
	Title       string
	Authors     []string
	Series      string
	SeriesIndex float64
	Tags        []string
	Publisher   string
	Languages   []string
	Rating      int // 0..10
	Comments    string
	Pages       int
	BG          color.RGBA // base page color (hue is rotated per page)
}

var samples = []sampleBook{
	{
		Title:       "The Incipit Sampler",
		Authors:     []string{"Ada Sample"},
		Series:      "Incipit Demo",
		SeriesIndex: 1,
		Tags:        []string{"Sample", "Demo"},
		Publisher:   "Incipit Press",
		Languages:   []string{"eng"},
		Rating:      10,
		Comments:    "A short procedurally-generated comic used to demo the Incipit CBZ reader. Every page is drawn on the fly — no external assets.",
		Pages:       6,
		BG:          color.RGBA{0x2b, 0x6c, 0xb0, 0xff},
	},
	{
		Title:       "Quick Start, Vol. 1",
		Authors:     []string{"Linus Demo", "Grace Page"},
		Series:      "Quick Start",
		SeriesIndex: 1,
		Tags:        []string{"Sample", "Tutorial"},
		Publisher:   "Incipit Press",
		Languages:   []string{"eng"},
		Rating:      8,
		Comments:    "Two authors, a series, and a handful of pages so listing, sorting and the reader all have something to chew on.",
		Pages:       5,
		BG:          color.RGBA{0x8e, 0x44, 0xad, 0xff},
	},
	{
		Title:     "Natural Order Test",
		Authors:   []string{"Sora Tester"},
		Tags:      []string{"Sample"},
		Publisher: "Incipit Press",
		Languages: []string{"jpn"},
		Rating:    6,
		Comments:  "Has 12 pages so the natural page sort (page2 before page10) is visibly exercised.",
		Pages:     12,
		BG:        color.RGBA{0x16, 0xa0, 0x85, 0xff},
	},
}

func main() {
	force := flag.Bool("force", false, "add the sample books even if the library already has books")
	reset := flag.Bool("reset", false, "delete every existing book first, then add the samples")
	flag.Parse()

	libraryPath := os.Getenv("INCIPIT_LIBRARY")
	if libraryPath == "" {
		libraryPath = "./library"
	}

	lib, err := calibre.Open(libraryPath, false)
	if err != nil {
		log.Fatalf("open library %q: %v", libraryPath, err)
	}
	defer lib.Close()

	ctx := context.Background()

	if *reset {
		// Page size large enough to cover any sample library in one pass.
		res, err := lib.ListBooks(ctx, calibre.ListOptions{Limit: 10000})
		if err != nil {
			log.Fatalf("list books: %v", err)
		}
		for _, b := range res.Books {
			if err := lib.DeleteBook(ctx, b.ID); err != nil {
				log.Fatalf("delete book #%d: %v", b.ID, err)
			}
			fmt.Printf("removed #%d %q\n", b.ID, b.Title)
		}
	} else if !*force {
		res, err := lib.ListBooks(ctx, calibre.ListOptions{Limit: 1})
		if err != nil {
			log.Fatalf("count books: %v", err)
		}
		if res.Total > 0 {
			fmt.Printf("library already has %d book(s); nothing to do (use -reset to replace them, -force to add the samples anyway)\n", res.Total)
			return
		}
	}

	for _, s := range samples {
		cbz, cover, err := buildCBZ(s)
		if err != nil {
			log.Fatalf("build %q: %v", s.Title, err)
		}
		book, err := lib.AddBook(ctx, calibre.AddBookInput{
			Title:       s.Title,
			Authors:     s.Authors,
			Series:      s.Series,
			SeriesIndex: s.SeriesIndex,
			Tags:        s.Tags,
			Publisher:   s.Publisher,
			Languages:   s.Languages,
			Rating:      s.Rating,
			Comments:    s.Comments,
			PubDate:     time.Now().UTC(),
			Format:      "CBZ",
			Data:        bytes.NewReader(cbz),
			Cover:       cover,
		})
		if err != nil {
			log.Fatalf("add %q: %v", s.Title, err)
		}
		fmt.Printf("added #%d %-22q by %v — %d pages → %s\n",
			book.ID, book.Title, s.Authors, s.Pages, book.Path)
	}

	fmt.Println("done. Start the server (make run) and open http://localhost:8080")
}

// buildCBZ renders the book's pages, zips them into a CBZ in natural-sort order
// and returns the archive bytes plus the first page encoded as a JPEG cover.
func buildCBZ(s sampleBook) (cbz []byte, cover []byte, err error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for i := 1; i <= s.Pages; i++ {
		img := drawPage(s, i)
		jpg, err := encodeJPEG(img)
		if err != nil {
			return nil, nil, err
		}
		// Deliberately zero-padded so the archive's lexical order already
		// matches reading order; the reader's natural sort handles either way.
		name := fmt.Sprintf("page-%02d.jpg", i)
		w, err := zw.Create(name)
		if err != nil {
			return nil, nil, err
		}
		if _, err := w.Write(jpg); err != nil {
			return nil, nil, err
		}
		if i == 1 {
			cover = jpg
		}
	}
	if err := zw.Close(); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), cover, nil
}

const (
	pageW = 800
	pageH = 1200
)

// drawPage renders a single placeholder comic page: a per-page tinted
// background, a header band with the title, and a large centered page number.
func drawPage(s sampleBook, page int) image.Image {
	// Rotate the hue a little per page so flipping through is visually obvious.
	bg := shade(s.BG, page, s.Pages)
	img := imaging.New(pageW, pageH, bg)

	// Header band.
	band := imaging.New(pageW, 140, color.NRGBA{0, 0, 0, 60})
	img = imaging.Paste(img, band, image.Pt(0, 0))

	img = pasteText(img, s.Title, 40, 52, 3, color.NRGBA{255, 255, 255, 255})
	img = pasteTextCentered(img, fmt.Sprintf("%d", page), pageH/2-90, 18, color.NRGBA{255, 255, 255, 235})
	img = pasteTextCentered(img, fmt.Sprintf("page %d of %d", page, s.Pages), pageH/2+130, 3, color.NRGBA{255, 255, 255, 220})
	img = pasteTextCentered(img, "incipit sample - generated", pageH-70, 2, color.NRGBA{255, 255, 255, 180})

	return img
}

// shade returns the base color brightened/darkened across the page range so the
// gradient from first to last page is noticeable.
func shade(base color.RGBA, page, total int) color.NRGBA {
	if total < 1 {
		total = 1
	}
	// factor sweeps 0.75 → 1.15 across the book.
	factor := 0.75 + 0.40*float64(page-1)/float64(maxInt(total-1, 1))
	clamp := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}
	return color.NRGBA{
		R: clamp(float64(base.R) * factor),
		G: clamp(float64(base.G) * factor),
		B: clamp(float64(base.B) * factor),
		A: 255,
	}
}

// pasteText draws s at (x,y) scaled by an integer factor using nearest-neighbor
// so the tiny built-in bitmap font reads as a chunky large label.
func pasteText(dst *image.NRGBA, s string, x, y, scale int, col color.NRGBA) *image.NRGBA {
	label := renderLabel(s, col)
	up := imaging.Resize(label, label.Bounds().Dx()*scale, label.Bounds().Dy()*scale, imaging.NearestNeighbor)
	return imaging.Overlay(dst, up, image.Pt(x, y), 1.0)
}

// pasteTextCentered horizontally centers the scaled label at vertical y.
func pasteTextCentered(dst *image.NRGBA, s string, y, scale int, col color.NRGBA) *image.NRGBA {
	label := renderLabel(s, col)
	w := label.Bounds().Dx() * scale
	up := imaging.Resize(label, w, label.Bounds().Dy()*scale, imaging.NearestNeighbor)
	return imaging.Overlay(dst, up, image.Pt((pageW-w)/2, y), 1.0)
}

// renderLabel draws s onto a tightly-cropped transparent RGBA using the 7x13
// built-in bitmap font.
func renderLabel(s string, col color.NRGBA) *image.RGBA {
	face := basicfont.Face7x13
	w := font.MeasureString(face, s).Ceil()
	if w < 1 {
		w = 1
	}
	h := face.Metrics().Height.Ceil() + 2
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(0, face.Metrics().Ascent.Ceil()),
	}
	d.DrawString(s)
	return img
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
