package calibre

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// metadata.opf is written into every book folder so the library round-trips
// with desktop Calibre and other tools. We emit an OPF 2.0 package with a
// Dublin Core metadata block, matching what Calibre expects to find.

type opfPackage struct {
	XMLName  xml.Name    `xml:"package"`
	Xmlns    string      `xml:"xmlns,attr"`
	Version  string      `xml:"version,attr"`
	UniqueID string      `xml:"unique-identifier,attr"`
	Metadata opfMetadata `xml:"metadata"`
}

type opfMetadata struct {
	XmlnsDC     string       `xml:"xmlns:dc,attr"`
	XmlnsOPF    string       `xml:"xmlns:opf,attr"`
	Title       string       `xml:"dc:title"`
	Creators    []opfCreator `xml:"dc:creator"`
	Identifiers []opfIdent   `xml:"dc:identifier"`
	Languages   []string     `xml:"dc:language"`
	Publisher   string       `xml:"dc:publisher,omitempty"`
	Date        string       `xml:"dc:date,omitempty"`
	Description string       `xml:"dc:description,omitempty"`
	Subjects    []string     `xml:"dc:subject"`
	Series      *opfMeta     `xml:"meta,omitempty"`
}

type opfCreator struct {
	Role string `xml:"opf:role,attr"`
	Sort string `xml:"opf:file-as,attr,omitempty"`
	Name string `xml:",chardata"`
}

type opfIdent struct {
	ID     string `xml:"id,attr,omitempty"`
	Scheme string `xml:"opf:scheme,attr,omitempty"`
	Value  string `xml:",chardata"`
}

type opfMeta struct {
	Name    string `xml:"name,attr"`
	Content string `xml:"content,attr"`
}

// buildOPF renders a metadata.opf document for a book.
func buildOPF(b *Book) ([]byte, error) {
	pkg := opfPackage{
		Xmlns:    "http://www.idpf.org/2007/opf",
		Version:  "2.0",
		UniqueID: "uuid_id",
		Metadata: opfMetadata{
			XmlnsDC:  "http://purl.org/dc/elements/1.1/",
			XmlnsOPF: "http://www.idpf.org/2007/opf",
			Title:    b.Title,
		},
	}
	for _, au := range b.Authors {
		pkg.Metadata.Creators = append(pkg.Metadata.Creators, opfCreator{
			Role: "aut", Sort: au.Sort, Name: au.Name,
		})
	}
	if b.UUID != "" {
		pkg.Metadata.Identifiers = append(pkg.Metadata.Identifiers, opfIdent{
			ID: "uuid_id", Scheme: "uuid", Value: b.UUID,
		})
	}
	for scheme, val := range b.Identifiers {
		pkg.Metadata.Identifiers = append(pkg.Metadata.Identifiers, opfIdent{
			Scheme: scheme, Value: val,
		})
	}
	pkg.Metadata.Languages = b.Languages
	if len(pkg.Metadata.Languages) == 0 {
		pkg.Metadata.Languages = []string{"und"}
	}
	if b.Publisher != nil {
		pkg.Metadata.Publisher = b.Publisher.Name
	}
	if !b.PubDate.IsZero() {
		pkg.Metadata.Date = b.PubDate.UTC().Format(time.RFC3339)
	}
	pkg.Metadata.Description = b.Comments
	for _, t := range b.Tags {
		pkg.Metadata.Subjects = append(pkg.Metadata.Subjects, t.Name)
	}
	if b.Series != nil {
		pkg.Metadata.Series = &opfMeta{Name: "calibre:series", Content: b.Series.Name}
	}

	body, err := xml.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal opf: %w", err)
	}
	var sb strings.Builder
	sb.WriteString(xml.Header)
	sb.Write(body)
	sb.WriteString("\n")
	return []byte(sb.String()), nil
}
