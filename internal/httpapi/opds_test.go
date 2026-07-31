package httpapi

import "testing"

func TestAcceptsJapanese(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", ""},
		{"ja", "ja"},
		{"ja-JP", "ja"},
		{"ja-JP,ja;q=0.9,en-US;q=0.8", "ja"},
		{"en-US,en;q=0.9", ""},
		{"en-US,en;q=0.9,ja;q=0.8", ""},
		{"en;q=0.7,ja;q=0.9", "ja"},
		{"*", ""},
		{"de-DE,de;q=0.9", ""},
	}
	for _, c := range cases {
		if got := acceptsJapanese(c.header); got != c.want {
			t.Errorf("acceptsJapanese(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}
