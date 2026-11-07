package wikiops

// Optional conversion is configured by trusted handlers, never JSON requests.
// Plain reads do not resolve, extract or execute Pandoc.
import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var ErrConverterUnavailable = errors.New("converter_unavailable")
var ErrConversion = errors.New("conversion_failed")

type pandocKey struct{}
type pdfKey struct{}
type pandocConfig struct {
	path, source string
	gate         chan struct{}
}

// WithPandoc accepts an absolute executable or trusted archive path. The
// converter is lazy and serial per handler lifetime; never use PATH discovery.
func WithPandoc(ctx context.Context, path, source string) context.Context {
	return context.WithValue(ctx, pandocKey{}, &pandocConfig{path, source, make(chan struct{}, 1)})
}

// WithPDF configures a trusted macOS helper; browser JSON cannot select it.
func WithPDF(ctx context.Context, path, source string) context.Context {
	return context.WithValue(ctx, pdfKey{}, &pandocConfig{path, source, make(chan struct{}, 1)})
}

type boundedOutput struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedOutput) Len() int      { return b.buffer.Len() }
func (b *boundedOutput) Bytes() []byte { return b.buffer.Bytes() }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		b.exceeded = true
		return 0, ErrExportLimit
	}
	return b.buffer.Write(p)
}
func convertDocx(ctx context.Context, html string) ([]byte, error) {
	config, _ := ctx.Value(pandocKey{}).(*pandocConfig)
	return convertLocal(ctx, html, config, "docx")
}
func convertPDF(ctx context.Context, html string) ([]byte, error) {
	if runtime.GOOS != "darwin" {
		return nil, ErrConverterUnavailable
	}
	config, _ := ctx.Value(pdfKey{}).(*pandocConfig)
	return convertLocal(ctx, html, config, "pdf")
}
func convertLocal(ctx context.Context, html string, config *pandocConfig, format string) ([]byte, error) {
	if config == nil || config.path == "" {
		return nil, ErrConverterUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	select {
	case config.gate <- struct{}{}:
		defer func() { <-config.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if !filepath.IsAbs(config.path) {
		return nil, ErrConverterUnavailable
	}
	tool, err := filepath.EvalSymlinks(config.path)
	if err != nil {
		return nil, ErrConverterUnavailable
	}
	source, err := filepath.EvalSymlinks(config.source)
	if err != nil {
		return nil, ErrConverterUnavailable
	}
	source, _ = filepath.Abs(source)
	rel, err := filepath.Rel(source, tool)
	if err != nil || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, ErrConverterUnavailable
	}
	info, err := os.Stat(tool)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<20 {
		return nil, ErrConverterUnavailable
	}
	tempBase, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return nil, ErrConverterUnavailable
	}
	tempBase, _ = filepath.Abs(tempBase)
	rel, err = filepath.Rel(source, tempBase)
	if err != nil || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, ErrConverterUnavailable
	}
	dir, err := os.MkdirTemp(tempBase, "wiki-docx-")
	if err != nil {
		return nil, ErrConversion
	}
	defer os.RemoveAll(dir)
	if strings.EqualFold(filepath.Ext(tool), ".zip") {
		if format != "docx" {
			return nil, ErrConverterUnavailable
		}
		archive, e := zip.OpenReader(tool)
		if e != nil {
			return nil, ErrConverterUnavailable
		}
		defer archive.Close()
		entry := "bin/pandoc"
		if runtime.GOOS == "windows" {
			entry += ".exe"
		}
		var binary *zip.File
		for _, file := range archive.File {
			if file.Name == entry {
				if binary != nil {
					return nil, ErrConverterUnavailable
				}
				binary = file
			}
		}
		if binary == nil || !binary.Mode().IsRegular() || binary.UncompressedSize64 > 256<<20 {
			return nil, ErrConverterUnavailable
		}
		input, e := binary.Open()
		if e != nil {
			return nil, ErrConverterUnavailable
		}
		tool = filepath.Join(dir, filepath.Base(entry))
		output, e := os.OpenFile(tool, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		if e != nil {
			input.Close()
			return nil, ErrConversion
		}
		n, copyErr := io.Copy(output, io.LimitReader(input, (256<<20)+1))
		closeErr := output.Close()
		input.Close()
		if copyErr != nil || closeErr != nil || n > 256<<20 {
			return nil, ErrConverterUnavailable
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	// Only self-contained sanitized HTML is admitted here. No filters, templates,
	// URLs, source paths or user-controlled command arguments reach the process.
	args := []string{"--sandbox", "--from=html", "--to=docx", "--output=-", "--data-dir=" + dir}
	if format == "pdf" {
		args = []string{"--html-stdin"}
	}
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	cmd.Env = []string{"HOME=" + dir, "XDG_CONFIG_HOME=" + dir, "TMPDIR=" + dir, "LANG=C.UTF-8"}
	cmd.WaitDelay = time.Second
	cmd.Stdin = strings.NewReader(html)
	output := &boundedOutput{limit: 16 << 20}
	cmd.Stdout = output
	// Do not return tool stderr, which can include source content or host paths.
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if output.exceeded {
			return nil, ErrExportLimit
		}
		return nil, ErrConversion
	}
	data := output.Bytes()
	if format == "pdf" {
		if len(data) < 50 || !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.HasSuffix(bytes.TrimSpace(data), []byte("%%EOF")) {
			return nil, ErrConversion
		}
		return data, nil
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrConversion
	}
	found := map[string]bool{}
	for _, file := range archive.File {
		found[file.Name] = true
	}
	if !found["[Content_Types].xml"] || !found["word/document.xml"] {
		return nil, ErrConversion
	}
	return data, nil
}
