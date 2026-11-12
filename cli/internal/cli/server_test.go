package cli

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// exitWith is what running a command that exits with code returns.
func exitWith(code int) error {
	return exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
}

func (ta *testApp) mustFail(t *testing.T, want string, args ...string) {
	t.Helper()
	if code := ta.Run(args); code != 1 {
		t.Fatalf("rembox %v: exit %d, want 1\nstdout:\n%s\nstderr:\n%s", args, code, ta.stdout, ta.stderr)
	}
	if !strings.Contains(ta.stderr.String(), want) {
		t.Errorf("rembox %v: stderr %q doesn't mention %q", args, ta.stderr, want)
	}
}

func TestWrap(t *testing.T) {
	text := strings.Repeat("word ", 40)
	for line := range strings.Lines(wrap(text, "    ")) {
		line = strings.TrimSuffix(line, "\n")
		if len(line) > 76 || !strings.HasPrefix(line, "    word") {
			t.Errorf("line %q", line)
		}
	}
	if got := wrap("", "  "); got != "  " {
		t.Errorf("wrap of nothing: %q", got)
	}
}
