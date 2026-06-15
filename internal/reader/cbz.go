// Package reader serves pages from CBZ (comic book ZIP) archives. Pages are
// extracted one at a time using the ZIP central directory — the whole archive
// is never decompressed — and optionally resized with an on-disk cache.
package reader

import (
	"archive/zip"
	"errors"
	"io"
	"path"
	"strings"
)

// ErrPageOutOfRange is returned when a requested page index does not exist.
var ErrPageOutOfRange = errors.New("reader: page index out of range")

// imageExts are the entry extensions treated as comic pages.
var imageExts = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
}

// Pages opens a CBZ and returns its image entry names in natural reading order.
// Opening the reader only parses the central directory at the end of the file,
// not the archive contents.
func Pages(cbzPath string) ([]string, error) {
	r, err := zip.OpenReader(cbzPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var names []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if _, ok := imageExts[strings.ToLower(path.Ext(f.Name))]; ok {
			names = append(names, f.Name)
		}
	}
	sortNatural(names)
	return names, nil
}

// ContentType returns the MIME type for a page entry name.
func ContentType(entryName string) string {
	if ct, ok := imageExts[strings.ToLower(path.Ext(entryName))]; ok {
		return ct
	}
	return "application/octet-stream"
}

// openEntry returns a reader for a single named entry within the CBZ. Only that
// entry is decompressed.
func openEntry(cbzPath, entryName string) (io.ReadCloser, *zip.ReadCloser, int64, error) {
	r, err := zip.OpenReader(cbzPath)
	if err != nil {
		return nil, nil, 0, err
	}
	for _, f := range r.File {
		if f.Name == entryName {
			rc, err := f.Open()
			if err != nil {
				r.Close()
				return nil, nil, 0, err
			}
			return rc, r, int64(f.UncompressedSize64), nil
		}
	}
	r.Close()
	return nil, nil, 0, ErrPageOutOfRange
}

// readEntry fully reads a single entry's bytes.
func readEntry(cbzPath, entryName string) ([]byte, error) {
	rc, zr, _, err := openEntry(cbzPath, entryName)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	defer rc.Close()
	return io.ReadAll(rc)
}
