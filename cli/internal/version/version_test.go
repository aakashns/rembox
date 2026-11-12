package version

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"v0.1.0", "0.1.0", 0},
		{"0.1.0", "0.2.0", -1},
		{"0.10.0", "0.9.0", 1},
		{"1.0.0", "0.99.99", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"1.0.0+build.5", "1.0.0", 0},
	}
	for _, tt := range tests {
		got, err := Compare(tt.a, tt.b)
		if err != nil {
			t.Errorf("Compare(%q, %q) error: %v", tt.a, tt.b, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCompareInvalid(t *testing.T) {
	for _, v := range []string{"dev", "", "1.0", "1.0.0.0", "01.0.0", "1.0.0-", "abc1234-dirty", "1.x.0"} {
		if Valid(v) {
			t.Errorf("Valid(%q) = true, want false", v)
		}
		if _, err := Compare(v, "1.0.0"); err == nil {
			t.Errorf("Compare(%q, 1.0.0) succeeded, want error", v)
		}
	}
}

func TestString(t *testing.T) {
	defer func(v, c, d string) { Version, Commit, Date = v, c, d }(Version, Commit, Date)

	Version, Commit, Date = "0.2.0", "abc1234", "2026-11-12T10:00:00Z"
	if got, want := String(), "rembox 0.2.0 (commit abc1234, 2026-11-12)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	Version, Commit, Date = "dev", "", ""
	if got, want := String(), "rembox dev"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
