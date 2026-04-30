package mdconv

import "testing"

// TestApplyMarpAcceptsAllHRForms pins the regex contract: html-to-markdown
// can emit `---`, `***`, or `* * *` for an <hr/>. If the library upgrades
// or its output changes, this catches the regression before users see a
// single un-split slide deck.
func TestApplyMarpAcceptsAllHRForms(t *testing.T) {
	cases := map[string]string{
		"dashes": "A\n\n---\n\nB\n",
		"stars":  "A\n\n***\n\nB\n",
		"spaced": "A\n\n* * *\n\nB\n",
		"mixed":  "A\n\n---\n\nB\n\n***\n\nC\n",
		"longer": "A\n\n----\n\nB\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			out := string(applyMarp([]byte(in)))
			want := "---\nmarp: true\n---\n\n"
			if got := out[:len(want)]; got != want {
				t.Errorf("missing front-matter: %q", got)
			}
			// Every input has 2+ segments and zero empty ones, so the
			// rebuilt body must contain exactly (segments-1) "\n\n---\n\n"
			// separators and no `***` / `* * *` should survive.
			if contains(out, "***") || contains(out, "* * *") {
				t.Errorf("non-canonical separator survived: %q", out)
			}
		})
	}
}

func TestInjectMarpSlideBreaksRewritesPageBreaks(t *testing.T) {
	in := []byte(`<p>A</p><h1 style="page-break-before:always; "></h1><p>B</p>` +
		`<div style="color:red;page-break-before: always"></div><p>C</p>`)
	got := string(injectMarpSlideBreaks(in))
	if c := countSubstr(got, "<hr/>"); c != 2 {
		t.Errorf("expected 2 <hr/> rewrites, got %d in: %s", c, got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func countSubstr(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
			i += len(sub) - 1
		}
	}
	return n
}
