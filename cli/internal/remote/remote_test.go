package remote

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
)

func TestScripts(t *testing.T) {
	for _, s := range []Script{Setup} {
		if s.Files[0] != "lib.sh" || s.Files[len(s.Files)-1] != s.Name {
			t.Errorf("%s is made of %q; want lib.sh first and itself last", s.Name, s.Files)
		}
		if !strings.HasPrefix(s.Body, "# lib.sh") || !strings.HasSuffix(s.Body, "main \"$@\" </dev/null\n") {
			t.Errorf("%s isn't lib.sh then a script ending with main", s.Name)
		}
	}
	// Every file is in a script, so none is left out by mistake.
	all, err := fs.Glob(files, "*.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all {
		if !slices.Contains(Setup.Files, f) {
			t.Errorf("%s isn't in any script", f)
		}
	}
	if !strings.HasSuffix(URL(), "/cli/internal/remote") {
		t.Errorf("URL: %s", URL())
	}
}
