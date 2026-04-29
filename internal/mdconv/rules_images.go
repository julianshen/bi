package mdconv

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 4 MiB per embedded image. Larger images are dropped — a typical
// embedded screenshot is well under 1 MiB; anything bigger is more
// likely a paste-in print resolution that nobody reads in Markdown.
const maxEmbedImageBytes = 4 * 1024 * 1024

var multiBlankRE = regexp.MustCompile(`\n{3,}`)

var imgRE = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

// applyImageMode rewrites every Markdown image reference per mode.
//
// resolveDir is the directory used to resolve relative `src` paths; the
// caller passes the directory the source HTML lived in.
func applyImageMode(md []byte, mode ImageMode, resolveDir string) []byte {
	switch mode {
	case ImagesDrop:
		out := imgRE.ReplaceAll(md, nil)
		return multiBlankRE.ReplaceAll(out, []byte("\n\n"))
	case ImagesEmbed:
		return imgRE.ReplaceAllFunc(md, func(match []byte) []byte {
			m := imgRE.FindSubmatch(match)
			alt, src := string(m[1]), string(m[2])
			if isDataURI(src) {
				return match
			}
			abs, ok := resolveImageSrc(resolveDir, src)
			if !ok {
				return nil // path-traversal attempt or unresolved → drop
			}
			// Bound peak memory: a document with many large images would
			// otherwise blow heap on (size + 4/3 base64) per image.
			if fi, err := os.Stat(abs); err != nil || fi.Size() > maxEmbedImageBytes {
				return nil
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				return nil
			}
			mime := http.DetectContentType(data)
			b64 := base64.StdEncoding.EncodeToString(data)
			var buf bytes.Buffer
			buf.WriteString("![")
			buf.WriteString(alt)
			buf.WriteString("](data:")
			buf.WriteString(mime)
			buf.WriteString(";base64,")
			buf.WriteString(b64)
			buf.WriteString(")")
			return buf.Bytes()
		})
	default:
		return md
	}
}

func isDataURI(s string) bool { return strings.HasPrefix(s, "data:") }

// resolveImageSrc joins resolveDir + src and refuses anything that escapes
// resolveDir via .. — blocks crafted document `<img src="../../etc/passwd">`
// from disclosing arbitrary host files in the rendered Markdown. Absolute
// paths are also rejected because LO HTML export should never emit them
// for embedded images.
//
// Lexical-only — symlinks are NOT followed. The threat model assumes LO's
// per-job temp directory contains no attacker-controlled symlinks (LO
// creates the dir fresh and emits its own image filenames). If that
// invariant changes, harden with filepath.EvalSymlinks before ReadFile.
func resolveImageSrc(resolveDir, src string) (string, bool) {
	if filepath.IsAbs(src) {
		return "", false
	}
	abs := filepath.Join(resolveDir, src)
	rel, err := filepath.Rel(resolveDir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return abs, true
}
