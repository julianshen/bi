package config

import "testing"

func TestPlatformDefaultsFor(t *testing.T) {
	cases := map[string]int{
		"linux":   2,
		"darwin":  2,
		"windows": 0,
		"":        0,
	}
	for goos, wantLen := range cases {
		got := platformDefaultsFor(goos)
		if len(got) != wantLen {
			t.Errorf("platformDefaultsFor(%q) = %v (len %d), want len %d", goos, got, len(got), wantLen)
		}
	}
}

func TestIsDirEmptyString(t *testing.T) {
	if isDir("") {
		t.Fatal(`isDir("") = true, want false`)
	}
}
