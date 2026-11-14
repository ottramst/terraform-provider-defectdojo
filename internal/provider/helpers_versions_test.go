package provider

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"3.5.103", "3.5.103", 0},
		{"3.5.103", "3.5.100", 1},
		{"3.4.304", "3.5.103", -1},
		{"3.5", "3.5.0", 0},
		{"3.6.1-dev", "3.6.1", 0},
		{"3.6.0-dev", "3.5.103", 1},
		{"3.10.0", "3.9.9", 1},
	}
	for _, tt := range tests {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
