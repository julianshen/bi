package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/julianshen/bi/internal/pngopts"
	"github.com/julianshen/bi/internal/worker"
)

// SubprocessConverter implements worker.Converter by spawning a child
// `bi convert` process per job. Each subprocess gets a fresh LO init and
// exits when the conversion finishes; if LO crashes inside the child the
// blast radius is one request, not the whole server. This is the workaround
// for issue #3 — LO/cgo state in the long-running bi server reliably
// crashes on POSTed documents while it works fine in a one-shot CLI.
//
// The contract with `cmd/bi/convert.go`:
//   - flags: -in, -out, -format, -page, -pages, -layout, -dpi, -password, -images, -marp, -ocr, -ocr-lang, -timeout
//   - success: exit 0, last stdout line is JSON {"mime":"...","total_pages":N}
//   - failure: exit non-zero, last stdout line is JSON {"error":"...","detail":"..."}
//   - the error string maps back to a worker sentinel via classifySubprocessErr.
type SubprocessConverter struct {
	BinPath      string        // path to the bi executable (typically os.Executable())
	LOKPath      string        // forwarded as $LOK_PATH to the child
	TmpDir       string        // where to stage output files; empty = system default
	Timeout      time.Duration // per-conversion ceiling; child also enforces this
	Metrics      *Metrics      // optional; observed for conversion duration and LOK errors
	OCRAvailable bool          // mirrors Deps.OCRAvailable; informational, set by serve at boot
}

// buildSubprocessArgs constructs the argv for `bi convert` from a
// worker.Job plus the resolved output path and timeout. Exposed for
// testing the flag contract.
func buildSubprocessArgs(job worker.Job, outPath string, timeout time.Duration) []string {
	args := []string{
		"convert",
		"-in", job.InPath,
		"-out", outPath,
		"-format", job.Format.String(),
	}
	if job.Format == worker.FormatPNG {
		if len(job.Pages) > 0 {
			args = append(args, "-pages", joinPages(job.Pages))
			layout := pngopts.Layout{Cols: job.GridCols, Rows: job.GridRows}
			if layout.Cols <= 0 {
				layout.Cols = len(job.Pages)
			}
			if layout.Rows <= 0 {
				layout.Rows = (len(job.Pages) + layout.Cols - 1) / layout.Cols
			}
			if layout.Cols > 0 && layout.Rows > 0 {
				args = append(args, "-layout", strconv.Itoa(layout.Cols)+"x"+strconv.Itoa(layout.Rows))
			}
		} else {
			args = append(args, "-page", strconv.Itoa(job.Page))
		}
		args = append(args, "-dpi", strconv.FormatFloat(job.DPI, 'f', -1, 64))
	}
	if job.Password != "" {
		args = append(args, "-password", job.Password)
	}
	if job.Format == worker.FormatMarkdown {
		mode := "embed"
		if job.MarkdownImages == worker.MarkdownImagesDrop {
			mode = "drop"
		}
		args = append(args, "-images", mode)
	}
	if job.MarkdownMarp {
		args = append(args, "-marp")
	}
	if job.Format == worker.FormatMarkdown {
		var ocrFlag string
		switch job.OCRMode {
		case worker.OCRAuto:
			ocrFlag = "auto"
		case worker.OCRAlways:
			ocrFlag = "always"
		case worker.OCRNever:
			ocrFlag = "never"
		default:
			ocrFlag = "auto"
		}
		args = append(args, "-ocr", ocrFlag)
		if job.OCRLang != "" {
			args = append(args, "-ocr-lang", job.OCRLang)
		}
	}
	if timeout > 0 {
		args = append(args, "-timeout", timeout.String())
	}
	return args
}

func joinPages(pages []int) string {
	var b strings.Builder
	for i, page := range pages {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(page))
	}
	return b.String()
}

// Run spawns `bi convert ...`, waits for it, and translates the child's
// outcome into a worker.Result / sentinel error.
func (s *SubprocessConverter) Run(ctx context.Context, job worker.Job) (worker.Result, error) {
	if s.BinPath == "" {
		return worker.Result{}, errors.New("subprocess: BinPath not set")
	}

	start := time.Now()
	outPath, err := outputTempPath(s.TmpDir, job.Format)
	if err != nil {
		return worker.Result{}, err
	}

	args := buildSubprocessArgs(job, outPath, s.Timeout)

	// LibreOffice creates a per-process user-profile directory (lu*.tmp)
	// in $TMPDIR (default /tmp). When many subprocess invocations share
	// the same /tmp, leftover profile dirs from previous runs (whose LO
	// crashed or didn't clean up cleanly) accumulate and confuse later
	// LO inits — issue #3 reproduces only when the server has handled
	// a prior conversion. Give each subprocess a fresh, isolated TMPDIR
	// and remove it after the child exits, so every LO init starts on
	// a clean slate.
	procTmp, err := os.MkdirTemp(s.TmpDir, "bi-proc-")
	if err != nil {
		return worker.Result{}, err
	}
	defer os.RemoveAll(procTmp)

	cmd := exec.CommandContext(ctx, s.BinPath, args...)
	cmd.Env = append(os.Environ(),
		"LOK_PATH="+s.LOKPath,
		"TMPDIR="+procTmp,
	)
	cmd.Stdin = nil
	// Start the child in a new session. The HTTP server has accumulated
	// signal-handler state (Go runtime + net/http + otelhttp) that leaks
	// across fork+exec via the shared signal mask. LO's init then trips
	// on a signal that's blocked at the OS level and crashes with the
	// generic "Unspecified Application Error". A new session resets
	// process-group state and lets LO install its own handlers cleanly.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if s.Metrics != nil {
		s.Metrics.Convert.WithLabelValues(job.Format.String()).Observe(time.Since(start).Seconds())
	}

	// The child's last stdout line is the JSON result envelope. Parse it
	// regardless of exit code: success has metadata, failure has error
	// classification. Any preceding stdout is logging, ignored here.
	envelope, parseErr := lastJSONLine(stdout.Bytes())
	if parseErr != nil {
		os.Remove(outPath)
		return worker.Result{}, fmt.Errorf("subprocess: parse output (run err=%v, stderr=%q): %w",
			runErr, trim(stderr.Bytes(), 256), parseErr)
	}

	if runErr != nil {
		os.Remove(outPath)
		if envelope.Error != "" {
			err := classifySubprocessErr(envelope.Error, envelope.Detail)
			if s.Metrics != nil {
				kind := worker.ErrorKind(err)
				if kind != "" {
					s.Metrics.LokErrorCounter.WithLabelValues(kind).Inc()
				}
			}
			return worker.Result{}, err
		}
		// No structured error — child exited abnormally without printing
		// the envelope (likely a crash). Surface stderr.
		if ctx.Err() != nil {
			return worker.Result{}, ctx.Err()
		}
		return worker.Result{}, fmt.Errorf("subprocess: %w (stderr=%q)",
			runErr, trim(stderr.Bytes(), 512))
	}

	return worker.Result{
		OutPath:    outPath,
		MIME:       envelope.MIME,
		TotalPages: envelope.TotalPages,
	}, nil
}

type subprocessEnvelope struct {
	MIME       string `json:"mime,omitempty"`
	TotalPages int    `json:"total_pages,omitempty"`
	Error      string `json:"error,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

func lastJSONLine(b []byte) (subprocessEnvelope, error) {
	lines := bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n"))
	if len(lines) == 0 {
		return subprocessEnvelope{}, errors.New("empty stdout")
	}
	var env subprocessEnvelope
	if err := json.Unmarshal(lines[len(lines)-1], &env); err != nil {
		return env, err
	}
	return env, nil
}

func outputTempPath(dir string, format worker.Format) (string, error) {
	var ext string
	switch format {
	case worker.FormatPDF:
		ext = ".pdf"
	case worker.FormatPNG:
		ext = ".png"
	case worker.FormatMarkdown:
		ext = ".md"
	default:
		return "", fmt.Errorf("unsupported format %d", format)
	}
	f, err := os.CreateTemp(dir, "bi-out-*"+ext)
	if err != nil {
		return "", err
	}
	path := f.Name()
	f.Close()
	// We just need a unique path; child will write to it.
	os.Remove(path)
	return path, nil
}

// classifySubprocessErr maps the child's error string back to a worker
// sentinel so the HTTP problem mapper produces the right status code. The
// strings here are the same contract written in cmd/bi/convert.go's
// classifyConvertErr.
func classifySubprocessErr(kind, detail string) error {
	switch kind {
	case "password-required":
		return wrap(worker.ErrPasswordRequired, detail)
	case "password-wrong":
		return wrap(worker.ErrWrongPassword, detail)
	case "unsupported-document":
		return wrap(worker.ErrUnsupportedFormat, detail)
	case "lok-unsupported":
		return wrap(worker.ErrLOKUnsupported, detail)
	case "page-out-of-range":
		return wrap(worker.ErrPageOutOfRange, detail)
	case "invalid-dpi":
		return wrap(worker.ErrInvalidDPI, detail)
	case "png-grid-too-large":
		return wrap(worker.ErrPNGGridTooLarge, detail)
	case "markdown-pipeline":
		return wrap(worker.ErrMarkdownConversion, detail)
	case "ocr-failed":
		return wrap(worker.ErrOCRFailed, detail)
	case "ocr-unavailable":
		return wrap(worker.ErrOCRUnavailable, detail)
	case "timeout":
		return context.DeadlineExceeded
	default:
		return fmt.Errorf("subprocess: %s: %s", kind, detail)
	}
}

func wrap(sentinel error, detail string) error {
	if detail == "" {
		return sentinel
	}
	return fmt.Errorf("%w: %s", sentinel, detail)
}

func trim(b []byte, max int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
