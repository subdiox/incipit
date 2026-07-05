package calibre

import "golang.org/x/text/unicode/norm"

// NFC returns s in Unicode NFC (composed) form. macOS stores filenames — and
// therefore titles derived from them — in NFD (decomposed), where "ド" is "ト"
// + a combining dakuten. That won't substring-match the NFC "ド" a user types,
// so we normalize every string on the way into metadata.db, keeping the library
// consistently NFC and aligned with cmoa/Amazon values (which are already NFC).
func NFC(s string) string { return norm.NFC.String(s) }

func nfcSlice(ss []string) []string {
	if ss == nil {
		return nil
	}
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = NFC(s)
	}
	return out
}

func nfcPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := NFC(*p)
	return &v
}

func nfcSlicePtr(p *[]string) *[]string {
	if p == nil {
		return nil
	}
	v := nfcSlice(*p)
	return &v
}
