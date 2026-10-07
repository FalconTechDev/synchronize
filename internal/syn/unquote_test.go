package syn

import "testing"

func TestUnquotePath(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`plain.txt`, `plain.txt`},
		{`"a file with spaces.txt"`, `a file with spaces.txt`},
		{`"\320\270\320\274\321\217.txt"`, `имя.txt`},
		{`"with \"quotes\".txt"`, `with "quotes".txt`},
		{`"back\\slash.txt"`, `back\slash.txt`},
		{`"tab\there.txt"`, "tab\there.txt"},
		{`"newline\nhere.txt"`, "newline\nhere.txt"},
		// Not quoted at all, and a lone quote inside: left alone rather than
		// mangled, because guessing would be worse than passing it through.
		{`no"quote.txt`, `no"quote.txt`},
		{`""`, ``},
		{`"`, `"`},
		{``, ``},
	} {
		if got := unquotePath(tc.in); got != tc.want {
			t.Errorf("unquotePath(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The whole point of the unquoting: a hook that watches a path must match
// the real name, not git's escaped rendering of it.
func TestHookPathsMatchUnicodeFilenames(t *testing.T) {
	h := Hook{Run: "x", Paths: []string{"док/**", "имя.txt"}}
	for _, changed := range [][]string{{"имя.txt"}, {"док/вложенный.txt"}} {
		if !h.Wants(changed) {
			t.Errorf("a hook watching Russian paths did not match %v", changed)
		}
	}
	if h.Wants([]string{`"\320\270\320\274\321\217.txt"`}) {
		t.Error("matched git's escaped form, which means the unquoting was skipped somewhere")
	}
}
