package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/julianshen/bi/internal/worker"
	"github.com/prometheus/client_golang/prometheus"
)

// fakeBinScript writes a tiny shell script that mimics `bi convert` for
// the SubprocessConverter contract. Returns the script path. The script
// reads -in / -out flags, optionally writes to -out, and prints the
// requested envelope (or error envelope) to stdout.
func fakeBinScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "bi-stub.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// argParser is a portable awk-free parser the stub scripts use to extract
// flag values. Defining the shell snippet here keeps tests readable.
const parseArgs = `
in=""; out=""; format=""
while [ $# -gt 0 ]; do
  case "$1" in
    -in) in="$2"; shift 2 ;;
    -out) out="$2"; shift 2 ;;
    -format) format="$2"; shift 2 ;;
    *) shift ;;
  esac
done
`

func TestSubprocessConverter_HappyPath(t *testing.T) {
	bin := fakeBinScript(t, "convert_main() {\n"+parseArgs+`
echo dummy-pdf-bytes > "$out"
echo '{"mime":"application/pdf","total_pages":3}'
}; shift; convert_main "$@"`)

	in := filepath.Join(t.TempDir(), "in.docx")
	if err := os.WriteFile(in, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &SubprocessConverter{BinPath: bin, Timeout: 5 * time.Second}
	res, err := c.Run(context.Background(), worker.Job{InPath: in, Format: worker.FormatPDF})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(res.OutPath) })

	if res.MIME != "application/pdf" {
		t.Errorf("MIME = %q", res.MIME)
	}
	if res.TotalPages != 3 {
		t.Errorf("TotalPages = %d", res.TotalPages)
	}
	if _, err := os.Stat(res.OutPath); err != nil {
		t.Errorf("OutPath does not exist: %v", err)
	}
}

func TestSubprocessConverter_PasswordRequired(t *testing.T) {
	bin := fakeBinScript(t, `echo '{"error":"password-required","detail":"locked"}'; exit 1`)
	c := &SubprocessConverter{BinPath: bin, Timeout: 5 * time.Second}
	_, err := c.Run(context.Background(), worker.Job{InPath: "/dev/null", Format: worker.FormatPDF})
	if !errors.Is(err, worker.ErrPasswordRequired) {
		t.Fatalf("err = %v, want ErrPasswordRequired", err)
	}
}

func TestSubprocessConverter_AllErrorMappings(t *testing.T) {
	cases := map[string]error{
		"password-required":    worker.ErrPasswordRequired,
		"password-wrong":       worker.ErrWrongPassword,
		"unsupported-document": worker.ErrUnsupportedFormat,
		"lok-unsupported":      worker.ErrLOKUnsupported,
		"page-out-of-range":    worker.ErrPageOutOfRange,
		"invalid-dpi":          worker.ErrInvalidDPI,
		"png-grid-too-large":   worker.ErrPNGGridTooLarge,
		"markdown-pipeline":    worker.ErrMarkdownConversion,
		"ocr-failed":           worker.ErrOCRFailed,
		"ocr-unavailable":      worker.ErrOCRUnavailable,
		"timeout":              context.DeadlineExceeded,
	}
	for kind, want := range cases {
		t.Run(kind, func(t *testing.T) {
			bin := fakeBinScript(t,
				`echo '{"error":"`+kind+`","detail":"x"}'; exit 1`)
			c := &SubprocessConverter{BinPath: bin, Timeout: 5 * time.Second}
			_, err := c.Run(context.Background(), worker.Job{InPath: "/dev/null", Format: worker.FormatPDF})
			if !errors.Is(err, want) {
				t.Errorf("err = %v, want %v", err, want)
			}
		})
	}
}

func TestSubprocessConverter_UnknownErrorKind(t *testing.T) {
	bin := fakeBinScript(t, `echo '{"error":"weird-kind","detail":"x"}'; exit 1`)
	c := &SubprocessConverter{BinPath: bin, Timeout: 5 * time.Second}
	_, err := c.Run(context.Background(), worker.Job{InPath: "/dev/null", Format: worker.FormatPDF})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestSubprocessConverter_NonzeroExitWithoutEnvelope(t *testing.T) {
	bin := fakeBinScript(t, `echo "not json"; exit 1`)
	c := &SubprocessConverter{BinPath: bin, Timeout: 5 * time.Second}
	_, err := c.Run(context.Background(), worker.Job{InPath: "/dev/null", Format: worker.FormatPDF})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestSubprocessConverter_BinPathRequired(t *testing.T) {
	c := &SubprocessConverter{}
	_, err := c.Run(context.Background(), worker.Job{Format: worker.FormatPDF})
	if err == nil {
		t.Fatal("want error for missing BinPath")
	}
}

func TestSubprocessConverter_ReturnsQueueFullWhenLimitReached(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	release := filepath.Join(dir, "release")
	bin := blockingPDFScript(t, started, release)

	in := filepath.Join(dir, "in.docx")
	if err := os.WriteFile(in, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &SubprocessConverter{
		BinPath:    bin,
		Timeout:    5 * time.Second,
		Workers:    1,
		QueueDepth: 0,
	}

	done := runPDFAsync(c, context.Background(), in)

	waitForFile(t, started)

	_, err := c.Run(context.Background(), worker.Job{InPath: in, Format: worker.FormatPDF})
	if !errors.Is(err, worker.ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}

	if err := os.WriteFile(release, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("first conversion: %v", err)
	}
}

func TestSubprocessConverter_QueueDepthAllowsOneWaitingRequest(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	release := filepath.Join(dir, "release")
	bin := blockingPDFScript(t, started, release)

	in := filepath.Join(dir, "in.docx")
	if err := os.WriteFile(in, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	c := &SubprocessConverter{
		BinPath:    bin,
		Timeout:    5 * time.Second,
		Workers:    1,
		QueueDepth: 1,
		Metrics:    metrics,
	}

	first := runPDFAsync(c, context.Background(), in)
	waitForFile(t, started)

	second := runPDFAsync(c, context.Background(), in)
	waitForCondition(t, "second request admitted", func() bool {
		return gaugeValue(t, reg, "bi_queue_depth") == 1
	})

	_, err := c.Run(context.Background(), worker.Job{InPath: in, Format: worker.FormatPDF})
	if !errors.Is(err, worker.ErrQueueFull) {
		t.Fatalf("third err = %v, want ErrQueueFull", err)
	}

	if err := os.WriteFile(release, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-first; err != nil {
		t.Fatalf("first conversion: %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("second conversion: %v", err)
	}
}

func TestSubprocessConverter_TimeoutCoversQueueWait(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	release := filepath.Join(dir, "release")
	bin := blockingPDFScript(t, started, release)

	in := filepath.Join(dir, "in.docx")
	if err := os.WriteFile(in, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &SubprocessConverter{
		BinPath:    bin,
		Timeout:    5 * time.Second,
		Workers:    1,
		QueueDepth: 1,
	}

	first := runPDFAsync(c, context.Background(), in)
	waitForFile(t, started)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	errc := runPDFAsync(c, ctx, in)
	select {
	case err := <-errc:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("queued err = %v, want DeadlineExceeded", err)
		}
	case <-time.After(300 * time.Millisecond):
		if err := os.WriteFile(release, []byte("ok"), 0o600); err != nil {
			t.Fatal(err)
		}
		<-first
		err := <-errc
		t.Fatalf("queued conversion did not time out while waiting; err after release = %v", err)
	}

	if err := os.WriteFile(release, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-first; err != nil {
		t.Fatalf("first conversion: %v", err)
	}
}

func TestSubprocessConverter_TimeoutKillsChild(t *testing.T) {
	dir := t.TempDir()
	bin := fakeBinScript(t, "convert_main() {\n"+parseArgs+`
sleep 1
}; shift; convert_main "$@"`)

	in := filepath.Join(dir, "in.docx")
	if err := os.WriteFile(in, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &SubprocessConverter{
		BinPath: bin,
		Timeout: 50 * time.Millisecond,
		Workers: 1,
	}

	_, err := c.Run(context.Background(), worker.Job{InPath: in, Format: worker.FormatPDF})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}

func TestSubprocessConverter_TimeoutKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	childPIDPath := filepath.Join(dir, "child.pid")
	bin := fakeBinScript(t, "convert_main() {\n"+parseArgs+`
sleep 5 &
echo $! > "`+childPIDPath+`"
wait
}; shift; convert_main "$@"`)

	in := filepath.Join(dir, "in.docx")
	if err := os.WriteFile(in, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &SubprocessConverter{
		BinPath: bin,
		Timeout: 500 * time.Millisecond,
		Workers: 1,
	}

	start := time.Now()
	_, err := c.Run(context.Background(), worker.Job{InPath: in, Format: worker.FormatPDF})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Run returned after %s, want prompt process-group cancellation", elapsed)
	}

	childPID := readPID(t, childPIDPath)
	t.Cleanup(func() {
		if processAlive(childPID) {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
		}
	})
	waitForCondition(t, "grandchild process exit", func() bool {
		return !processAlive(childPID)
	})
}

func TestSubprocessConverter_InstrumentsLimiterMetrics(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	release := filepath.Join(dir, "release")
	bin := blockingPDFScript(t, started, release)

	in := filepath.Join(dir, "in.docx")
	if err := os.WriteFile(in, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	c := &SubprocessConverter{
		BinPath:    bin,
		Timeout:    5 * time.Second,
		Workers:    1,
		QueueDepth: 1,
		Metrics:    metrics,
	}

	first := runPDFAsync(c, context.Background(), in)
	waitForFile(t, started)
	waitForCondition(t, "busy metric", func() bool {
		return gaugeValue(t, reg, "bi_worker_busy") == 1
	})

	second := runPDFAsync(c, context.Background(), in)
	waitForCondition(t, "queue depth metric", func() bool {
		return gaugeValue(t, reg, "bi_queue_depth") == 1
	})

	if err := os.WriteFile(release, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-first; err != nil {
		t.Fatalf("first conversion: %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("second conversion: %v", err)
	}
	if got := histogramSampleCount(t, reg, "bi_queue_wait_seconds"); got == 0 {
		t.Fatal("queue wait histogram had no observations")
	}
	if got := gaugeValue(t, reg, "bi_worker_busy"); got != 0 {
		t.Fatalf("busy metric after completion = %v, want 0", got)
	}
}

func gaugeValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if gauge := metric.GetGauge(); gauge != nil {
				return gauge.GetValue()
			}
		}
	}
	return 0
}

func histogramSampleCount(t *testing.T, reg *prometheus.Registry, name string) uint64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		var total uint64
		for _, metric := range family.GetMetric() {
			if hist := metric.GetHistogram(); hist != nil {
				total += hist.GetSampleCount()
			}
		}
		return total
	}
	return 0
}

func blockingPDFScript(t *testing.T, started, release string) string {
	t.Helper()
	return fakeBinScript(t, "convert_main() {\n"+parseArgs+`
echo started >> "`+started+`"
while [ ! -f "`+release+`" ]; do sleep 0.01; done
echo dummy-pdf-bytes > "$out"
echo '{"mime":"application/pdf","total_pages":1}'
}; shift; convert_main "$@"`)
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func runPDFAsync(c *SubprocessConverter, ctx context.Context, in string) <-chan error {
	done := make(chan error, 1)
	go func() {
		res, err := c.Run(ctx, worker.Job{InPath: in, Format: worker.FormatPDF})
		if res.OutPath != "" {
			os.Remove(res.OutPath)
		}
		done <- err
	}()
	return done
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	waitForCondition(t, path, func() bool {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		return false
	})
}

func waitForCondition(t *testing.T, name string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", name)
}

func TestSubprocessConverter_OutputTempPath(t *testing.T) {
	cases := map[worker.Format]string{
		worker.FormatPDF:      ".pdf",
		worker.FormatPNG:      ".png",
		worker.FormatMarkdown: ".md",
	}
	for f, ext := range cases {
		got, err := outputTempPath("", f)
		if err != nil {
			t.Fatalf("format %v: %v", f, err)
		}
		if filepath.Ext(got) != ext {
			t.Errorf("format %v: ext = %q, want %q", f, filepath.Ext(got), ext)
		}
	}
}

func TestSubprocessConverter_OutputTempPathRejectsUnknownFormat(t *testing.T) {
	_, err := outputTempPath("", worker.Format(99))
	if err == nil {
		t.Fatal("want error")
	}
}

func TestSubprocessConverter_TrimTruncates(t *testing.T) {
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'x'
	}
	got := trim(long, 256)
	if len(got) != 256+len("…") {
		t.Errorf("trim len = %d, want %d", len(got), 256+len("…"))
	}
}

func TestSubprocessConverter_WrapNoDetail(t *testing.T) {
	if wrap(worker.ErrPasswordRequired, "") != worker.ErrPasswordRequired {
		t.Error("wrap with empty detail should return sentinel as-is")
	}
}

func TestSubprocessConverter_MarpFlagForwarded(t *testing.T) {
	bin := fakeBinScript(t, `
out=""
next_out=0
saw_marp=0
for a in "$@"; do
  if [ "$next_out" = "1" ]; then out="$a"; next_out=0; continue; fi
  case "$a" in
    -out) next_out=1 ;;
    -marp) saw_marp=1 ;;
  esac
done
: > "$out"
if [ "$saw_marp" = "1" ]; then
  echo '{"mime":"text/markdown","total_pages":0}'
else
  echo '{"error":"internal","detail":"-marp flag not forwarded"}'
  exit 1
fi
`)
	in := filepath.Join(t.TempDir(), "in.pptx")
	os.WriteFile(in, []byte("x"), 0o600)
	c := &SubprocessConverter{BinPath: bin, Timeout: 5 * time.Second}
	res, err := c.Run(context.Background(), worker.Job{
		InPath: in, Format: worker.FormatMarkdown, MarkdownMarp: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(res.OutPath) })
	if res.MIME != "text/markdown" {
		t.Errorf("MIME = %q", res.MIME)
	}
}

// TestSubprocessConverter_MarpFlagOmittedWhenFalse pins the negative
// half of the contract: a markdown job with MarkdownMarp=false must
// not pass -marp. Without this, an "always-append" bug would still
// satisfy TestSubprocessConverter_MarpFlagForwarded.
func TestSubprocessConverter_MarpFlagOmittedWhenFalse(t *testing.T) {
	bin := fakeBinScript(t, `
out=""
next_out=0
saw_marp=0
for a in "$@"; do
  if [ "$next_out" = "1" ]; then out="$a"; next_out=0; continue; fi
  case "$a" in
    -out) next_out=1 ;;
    -marp) saw_marp=1 ;;
  esac
done
: > "$out"
if [ "$saw_marp" = "1" ]; then
  echo '{"error":"internal","detail":"-marp flag forwarded when MarkdownMarp=false"}'
  exit 1
else
  echo '{"mime":"text/markdown","total_pages":0}'
fi
`)
	in := filepath.Join(t.TempDir(), "in.docx")
	os.WriteFile(in, []byte("x"), 0o600)
	c := &SubprocessConverter{BinPath: bin, Timeout: 5 * time.Second}
	res, err := c.Run(context.Background(), worker.Job{
		InPath: in, Format: worker.FormatMarkdown, MarkdownMarp: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(res.OutPath) })
}

func TestBuildSubprocessArgsForwardsOCR(t *testing.T) {
	args := buildSubprocessArgs(worker.Job{
		Format:  worker.FormatMarkdown,
		InPath:  "/tmp/in.pdf",
		OCRMode: worker.OCRAlways,
		OCRLang: "jpn",
	}, "/tmp/out.md", 5*time.Second)

	wantPairs := [][2]string{
		{"-ocr", "always"},
		{"-ocr-lang", "jpn"},
	}
	for _, p := range wantPairs {
		if !argHasPair(args, p[0], p[1]) {
			t.Errorf("missing %v in args=%v", p, args)
		}
	}
}

func TestBuildSubprocessArgsAutoLangOmitted(t *testing.T) {
	// When OCRLang is empty, no -ocr-lang flag is emitted; the child's
	// own default is used instead.
	args := buildSubprocessArgs(worker.Job{
		Format:  worker.FormatMarkdown,
		InPath:  "/tmp/in.pdf",
		OCRMode: worker.OCRAuto,
		OCRLang: "",
	}, "/tmp/out.md", 0)
	for _, a := range args {
		if a == "-ocr-lang" {
			t.Errorf("unexpected -ocr-lang in args=%v", args)
		}
	}
}

func argHasPair(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func TestSubprocessConverter_PNGFlagsForwarded(t *testing.T) {
	bin := fakeBinScript(t, `
out=""
next=0
for a in "$@"; do
  if [ "$next" = "1" ]; then out="$a"; next=0; continue; fi
  if [ "$a" = "-out" ]; then next=1; fi
done
: > "$out"
echo '{"mime":"image/png","total_pages":7}'
`)
	in := filepath.Join(t.TempDir(), "in.docx")
	os.WriteFile(in, []byte("x"), 0o600)
	c := &SubprocessConverter{BinPath: bin, Timeout: 5 * time.Second}
	res, err := c.Run(context.Background(), worker.Job{
		InPath: in, Format: worker.FormatPNG, Page: 4, DPI: 1.5, Password: "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(res.OutPath) })
	if res.TotalPages != 7 {
		t.Errorf("TotalPages = %d, want 7", res.TotalPages)
	}
}

func TestBuildSubprocessArgsForwardsPNGPagesAndLayout(t *testing.T) {
	args := buildSubprocessArgs(worker.Job{
		Format:   worker.FormatPNG,
		InPath:   "/tmp/in.pptx",
		Pages:    []int{0, 2, 4},
		GridCols: 2,
		GridRows: 2,
		DPI:      1.5,
	}, "/tmp/out.png", 5*time.Second)

	for _, p := range [][2]string{
		{"-pages", "0,2,4"},
		{"-layout", "2x2"},
		{"-dpi", "1.5"},
	} {
		if !argHasPair(args, p[0], p[1]) {
			t.Errorf("missing %v in args=%v", p, args)
		}
	}
	for _, a := range args {
		if a == "-page" {
			t.Fatalf("unexpected -page in multi-page args=%v", args)
		}
	}
}
