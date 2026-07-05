package metadata

import "strings"

// NormalizeISBN extracts a plausible ISBN from s. cmoa renders the ISBN as plain
// digits (e.g. "9784803005950"), but callers may pass a whole table cell that
// also contains the "：" separator, so we keep only ISBN characters. It returns a
// canonical 13-digit (978/979-prefixed) or 10-character (trailing 'X' allowed)
// ISBN, or "" when s holds nothing that looks like one.
func NormalizeISBN(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == 'X' || r == 'x':
			b.WriteByte('X') // ISBN-10 check character
		}
	}
	d := b.String()
	switch len(d) {
	case 13:
		if (strings.HasPrefix(d, "978") || strings.HasPrefix(d, "979")) && !strings.Contains(d, "X") {
			return d
		}
	case 10:
		// Only the final character may be 'X'.
		if !strings.Contains(d[:9], "X") {
			return d
		}
	}
	return ""
}

// ISBNToASIN converts a 978-prefixed ISBN-13 to its ISBN-10 form, which — for
// print books — is the Amazon ASIN. It recomputes the ISBN-10 check character
// from the 9 payload digits. 979-prefixed ISBN-13s have no ISBN-10 equivalent
// and return ""; a value already in ISBN-10 form is returned as-is. Note this
// does not apply to Kindle-only editions, whose ASINs (B...) are unrelated.
func ISBNToASIN(isbn string) string {
	isbn = NormalizeISBN(isbn)
	switch len(isbn) {
	case 10:
		return isbn
	case 13:
		if !strings.HasPrefix(isbn, "978") {
			return "" // 979 range has no ISBN-10
		}
		body := isbn[3:12] // 9 payload digits (drop the 978 prefix and old check digit)
		sum := 0
		for i := 0; i < len(body); i++ {
			sum += (10 - i) * int(body[i]-'0')
		}
		check := (11 - sum%11) % 11
		if check == 10 {
			return body + "X"
		}
		return body + string(rune('0'+check))
	}
	return ""
}
