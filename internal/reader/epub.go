package reader

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"path"
	"strings"

	"github.com/disintegration/imaging"
)

// ErrNoCover is returned when a book has no extractable cover image.
var ErrNoCover = errors.New("reader: no cover")

// EpubCoverJPEG extracts an EPUB's cover image and returns it as a JPEG bounded
// to maxWidth. EPUB files are ZIP containers; the cover is located via the OPF
// package document referenced from META-INF/container.xml.
func (s *Service) EpubCoverJPEG(epubPath string, maxWidth int) ([]byte, error) {
	raw, err := epubCoverBytes(epubPath)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
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

func epubCoverBytes(epubPath string) ([]byte, error) {
	zr, err := zip.OpenReader(epubPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}

	opfPath, err := opfPathFromContainer(files)
	if err != nil {
		return nil, err
	}
	opf, ok := files[opfPath]
	if !ok {
		return nil, ErrNoCover
	}
	href, err := coverHref(opf)
	if err != nil {
		return nil, err
	}
	// Cover href is relative to the OPF document's directory.
	coverName := path.Join(path.Dir(opfPath), href)
	cf, ok := files[coverName]
	if !ok {
		return nil, ErrNoCover
	}
	rc, err := cf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func opfPathFromContainer(files map[string]*zip.File) (string, error) {
	cf, ok := files["META-INF/container.xml"]
	if !ok {
		return "", ErrNoCover
	}
	rc, err := cf.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var c struct {
		Rootfiles struct {
			Rootfile []struct {
				FullPath string `xml:"full-path,attr"`
			} `xml:"rootfile"`
		} `xml:"rootfiles"`
	}
	if err := xml.NewDecoder(rc).Decode(&c); err != nil {
		return "", err
	}
	if len(c.Rootfiles.Rootfile) == 0 || c.Rootfiles.Rootfile[0].FullPath == "" {
		return "", ErrNoCover
	}
	return c.Rootfiles.Rootfile[0].FullPath, nil
}

// coverHref resolves the cover image href within an OPF package document,
// trying (in order): the EPUB2 <meta name="cover"> id, an EPUB3
// properties="cover-image" manifest item, then any image item that looks like a
// cover.
func coverHref(opf *zip.File) (string, error) {
	rc, err := opf.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var pkg struct {
		Metadata struct {
			Meta []struct {
				Name    string `xml:"name,attr"`
				Content string `xml:"content,attr"`
			} `xml:"meta"`
		} `xml:"metadata"`
		Manifest struct {
			Item []struct {
				ID         string `xml:"id,attr"`
				Href       string `xml:"href,attr"`
				MediaType  string `xml:"media-type,attr"`
				Properties string `xml:"properties,attr"`
			} `xml:"item"`
		} `xml:"manifest"`
	}
	if err := xml.NewDecoder(rc).Decode(&pkg); err != nil {
		return "", err
	}

	coverID := ""
	for _, m := range pkg.Metadata.Meta {
		if strings.EqualFold(m.Name, "cover") && m.Content != "" {
			coverID = m.Content
			break
		}
	}
	if coverID != "" {
		for _, it := range pkg.Manifest.Item {
			if it.ID == coverID && it.Href != "" {
				return it.Href, nil
			}
		}
	}
	for _, it := range pkg.Manifest.Item {
		if strings.Contains(it.Properties, "cover-image") && it.Href != "" {
			return it.Href, nil
		}
	}
	for _, it := range pkg.Manifest.Item {
		if strings.HasPrefix(it.MediaType, "image/") &&
			(strings.Contains(strings.ToLower(it.ID), "cover") ||
				strings.Contains(strings.ToLower(it.Href), "cover")) {
			return it.Href, nil
		}
	}
	return "", ErrNoCover
}
