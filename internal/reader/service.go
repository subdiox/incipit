package reader

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"

	// Register image decoders so image.Decode handles every page format.
	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// Service renders CBZ pages with an on-disk cache for resized output.
type Service struct {
	cacheDir string
}

// NewService creates a reader service caching resized images under cacheDir.
func NewService(cacheDir string) *Service {
	_ = os.MkdirAll(cacheDir, 0o755)
	return &Service{cacheDir: cacheDir}
}

// RenderedPage is a page ready to serve over HTTP.
type RenderedPage struct {
	Data        []byte
	ContentType string
	ETag        string
}

// RenderPage returns page `index` from the (already-listed) pages slice. When
// maxWidth > 0 and the page is wider, it is downscaled and JPEG-encoded with an
// on-disk cache keyed by the archive's mtime; the original bytes are streamed
// otherwise. mtime should be the CBZ file's modification time (caller stats it
// once and reuses it for cache validity).
func (s *Service) RenderPage(cbzPath string, pages []string, index, maxWidth int, mtime int64) (*RenderedPage, error) {
	if index < 0 || index >= len(pages) {
		return nil, ErrPageOutOfRange
	}
	entry := pages[index]

	if maxWidth <= 0 {
		data, err := readEntry(cbzPath, entry)
		if err != nil {
			return nil, err
		}
		return &RenderedPage{Data: data, ContentType: ContentType(entry), ETag: key(cbzPath, entry, 0, mtime)}, nil
	}

	k := key(cbzPath, entry, maxWidth, mtime)
	cachePath := filepath.Join(s.cacheDir, k+".jpg")
	if data, err := os.ReadFile(cachePath); err == nil {
		return &RenderedPage{Data: data, ContentType: "image/jpeg", ETag: k}, nil
	}

	jpegData, err := s.resizeToJPEG(cbzPath, entry, maxWidth)
	if err != nil {
		return nil, err
	}
	writeCache(cachePath, jpegData)
	return &RenderedPage{Data: jpegData, ContentType: "image/jpeg", ETag: k}, nil
}

// FirstPageJPEG renders the first page of a CBZ as a JPEG bounded to maxWidth,
// suitable for generating a cover thumbnail.
func (s *Service) FirstPageJPEG(cbzPath string, maxWidth int) ([]byte, error) {
	pages, err := Pages(cbzPath)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, ErrPageOutOfRange
	}
	return s.resizeToJPEG(cbzPath, pages[0], maxWidth)
}

func (s *Service) resizeToJPEG(cbzPath, entry string, maxWidth int) ([]byte, error) {
	raw, err := readEntry(cbzPath, entry)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode page %q: %w", entry, err)
	}
	if maxWidth > 0 && img.Bounds().Dx() > maxWidth {
		img = imaging.Resize(img, maxWidth, 0, imaging.Lanczos)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// key derives a stable cache key / ETag from the archive identity, entry, width
// and mtime, so a modified CBZ invalidates cached renders.
func key(cbzPath, entry string, width int, mtime int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d", cbzPath, entry, width, mtime)))
	return hex.EncodeToString(h[:16])
}

// writeCache writes data to path atomically (temp + rename), ignoring errors —
// the cache is an optimization, not a source of truth.
func writeCache(path string, data []byte) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	tmp.Close()
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
	}
}
