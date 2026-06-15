package calibre

import (
	"fmt"
	"strings"
	"unicode"
)

// maxComponent is the per-path-component character cap. Calibre truncates path
// components to keep within filesystem limits; 40 is a safe, Calibre-like
// default that leaves room for the " (id)" suffix and file extensions.
const maxComponent = 40

// sanitizeComponent makes a string safe to use as a single path component on
// the common filesystems (ext4/APFS/NTFS): illegal characters become '_',
// whitespace is collapsed, and leading/trailing dots and spaces are trimmed
// (Windows rejects those). The result is truncated to maxComponent runes.
func sanitizeComponent(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20: // control characters
			b.WriteRune(' ')
		case strings.ContainsRune(`\/:*?"<>|`, r):
			b.WriteRune('_')
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	// Collapse runs of spaces.
	out := strings.Join(strings.Fields(b.String()), " ")
	out = truncateRunes(out, maxComponent)
	out = strings.TrimRight(out, " .")
	out = strings.TrimSpace(out)
	if out == "" {
		out = "Unknown"
	}
	return out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// bookRelPath computes a book's folder path (forward-slash, relative to the
// library root) and the basename Calibre uses for the book's files, following
// the "<Author>/<Title> (<id>)" + "<Title> - <Author>" conventions.
func bookRelPath(id int64, title string, authors []string) (relPath, fileBase string) {
	author := "Unknown"
	if len(authors) > 0 && strings.TrimSpace(authors[0]) != "" {
		author = authors[0]
	}
	authorDir := sanitizeComponent(author)
	titleDir := sanitizeComponent(title) + fmt.Sprintf(" (%d)", id)
	relPath = authorDir + "/" + titleDir

	fileBase = sanitizeComponent(title + " - " + author)
	return relPath, fileBase
}
