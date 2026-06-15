package reader

import (
	"sort"
	"strings"
)

// sortNatural sorts names so that embedded numbers compare numerically, e.g.
// "page2.jpg" < "page10.jpg". Comic archives rely on this ordering.
func sortNatural(names []string) {
	sort.SliceStable(names, func(i, j int) bool {
		return naturalLess(strings.ToLower(names[i]), strings.ToLower(names[j]))
	})
}

func naturalLess(a, b string) bool {
	ia, ib := 0, 0
	for ia < len(a) && ib < len(b) {
		da := isDigit(a[ia])
		db := isDigit(b[ib])
		if da && db {
			// Compare two numeric runs by value, ignoring leading zeros.
			na, va := numRun(a, ia)
			nb, vb := numRun(b, ib)
			if va != vb {
				return va < vb
			}
			ia, ib = na, nb
			continue
		}
		if a[ia] != b[ib] {
			return a[ia] < b[ib]
		}
		ia++
		ib++
	}
	return len(a)-ia < len(b)-ib
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// numRun returns the index past the numeric run starting at i and its value.
// Values are clamped so extremely long digit runs don't overflow.
func numRun(s string, i int) (int, uint64) {
	var v uint64
	for i < len(s) && isDigit(s[i]) {
		if v < (1 << 63) {
			v = v*10 + uint64(s[i]-'0')
		}
		i++
	}
	return i, v
}
