package calibre

import (
	"crypto/rand"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"
	"sync"

	sqlite "modernc.org/sqlite"
)

// Calibre registers a handful of application-defined SQL functions on every
// connection. Its triggers reference title_sort and uuid4, so any process that
// writes to metadata.db must provide them or inserts abort. We register Go
// implementations globally; modernc.org/sqlite makes them available to all
// connections opened afterwards.
//
// registerOnce guards against double registration (the driver errors if a name
// is registered twice), which matters because tests open many libraries.
var registerOnce sync.Once

func registerSQLFunctions() {
	registerOnce.Do(func() {
		sqlite.MustRegisterDeterministicScalarFunction("title_sort", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			return TitleSort(asString(args[0])), nil
		})
		sqlite.MustRegisterDeterministicScalarFunction("author_to_author_sort", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			return AuthorSort(asString(args[0])), nil
		})
		sqlite.MustRegisterScalarFunction("uuid4", 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			return UUID4(), nil
		})
	})
}

func asString(v driver.Value) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

// leadingArticle matches the English articles Calibre strips by default when
// computing a title's sort key.
var leadingArticle = regexp.MustCompile(`(?i)^(A|The|An)\s+`)

// TitleSort mirrors Calibre's default title_sort: a leading article is moved to
// the end so "The Hobbit" sorts as "Hobbit, The".
func TitleSort(title string) string {
	t := strings.TrimSpace(title)
	loc := leadingArticle.FindStringSubmatchIndex(t)
	if loc == nil {
		return t
	}
	article := t[loc[2]:loc[3]]
	rest := strings.TrimSpace(t[loc[1]:])
	if rest == "" {
		return t
	}
	return rest + ", " + article
}

// authorSuffixes are tokens that should not be treated as a family name when
// they appear last (matching Calibre's author_to_author_sort copywords).
var authorSuffixes = map[string]bool{
	"jr": true, "jr.": true, "sr": true, "sr.": true,
	"i": true, "ii": true, "iii": true, "iv": true, "v": true,
	"md": true, "phd": true, "esq": true, "esq.": true,
}

// AuthorSort mirrors Calibre's author_to_author_sort: "Firstname Lastname"
// becomes "Lastname, Firstname". A name that already contains a comma is left
// untouched, and a trailing suffix (Jr, III, ...) stays attached to the family
// name.
func AuthorSort(name string) string {
	n := strings.TrimSpace(name)
	if n == "" || strings.Contains(n, ",") {
		return n
	}
	tokens := strings.Fields(n)
	if len(tokens) == 1 {
		return tokens[0]
	}

	familyIdx := len(tokens) - 1
	suffix := ""
	if authorSuffixes[strings.ToLower(tokens[familyIdx])] && familyIdx > 0 {
		suffix = " " + tokens[familyIdx]
		familyIdx--
	}
	family := tokens[familyIdx] + suffix
	given := strings.Join(tokens[:familyIdx], " ")
	if given == "" {
		return family
	}
	return family + ", " + given
}

// UUID4 returns a random RFC 4122 version-4 UUID string, matching the format
// Calibre stores in books.uuid.
func UUID4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is fatal in practice; degrade to a zero UUID
		// rather than panic inside a SQL trigger.
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
