package ocr

import (
	"context"
	"errors"
	"testing"
)

func TestValidateLangsAllowlist(t *testing.T) {
	good := []string{"eng", "jpn", "chi_sim", "chi_tra", "auto", "all", "eng+jpn", "eng+jpn+chi_sim+chi_tra"}
	for _, in := range good {
		if err := ValidateLangs(in); err != nil {
			t.Errorf("ValidateLangs(%q) = %v, want nil", in, err)
		}
	}
	bad := []string{"", "fra", "eng+fra", "eng+", "+eng", "eng eng", "ENG", "auto+eng"}
	for _, in := range bad {
		if err := ValidateLangs(in); err == nil {
			t.Errorf("ValidateLangs(%q) = nil, want error", in)
		}
	}
}

func TestResolveLangs(t *testing.T) {
	cases := map[string]string{
		"eng":     "eng",
		"jpn":     "jpn",
		"eng+jpn": "eng+jpn",
		"all":     "eng+jpn+chi_sim+chi_tra",
		"auto":    "",
	}
	for in, want := range cases {
		got, err := ResolveLangs(in)
		if err != nil {
			t.Fatalf("ResolveLangs(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("ResolveLangs(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrSentinels(t *testing.T) {
	if !errors.Is(ErrUnavailable, ErrUnavailable) {
		t.Fatal("ErrUnavailable is not its own value")
	}
	_ = context.Background()
}
