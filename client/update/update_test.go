package update

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		cur, lat string
		want     bool
	}{
		{"0.1.0", "0.2.0", true},
		{"0.2.0", "0.1.0", false},
		{"0.1.0", "0.1.0", false},
		{"1.0.0", "1.0.1", true},
		{"1.2.3", "1.2.3", false},
		{"0.0.0-dev", "0.1.0", true}, // dev always updates
		{"v1.0.0", "v1.1.0", true},   // tolerate leading v
		{"1.0.0", "", false},         // no latest
	}
	for _, c := range cases {
		if got := IsNewer(c.cur, c.lat); got != c.want {
			t.Errorf("IsNewer(%q,%q) = %v, want %v", c.cur, c.lat, got, c.want)
		}
	}
}
