package remote

import (
	"strings"
	"testing"
)

func TestScripts(t *testing.T) {
	for _, s := range []Script{Setup} {
		if !strings.HasPrefix(s.Body, "# lib.sh") || !strings.HasSuffix(s.Body, "main \"$@\" </dev/null\n") {
			t.Errorf("%s isn't lib.sh then a script ending with main", s.Name)
		}
		if !strings.HasSuffix(s.URL(), "/cli/internal/remote/"+s.Name) {
			t.Errorf("%s's URL: %s", s.Name, s.URL())
		}
	}
}
