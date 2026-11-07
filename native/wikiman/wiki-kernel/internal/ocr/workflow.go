package ocr

import (
	"context"
	"fmt"
	"html"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DeviceOptions is called explicitly by a device/compatibility bootstrap, never
// on read/import. Context owners receive the resulting copied value, not getenv.
func DeviceOptions(lookup func(string) string, standard bool) (Options, error) {
	o := Defaults()
	o.Enabled = standard
	if lookup == nil {
		return o, nil
	}
	if s := lookup("WIKI_TESSERACT_ENABLED"); s != "" {
		v, e := strconv.ParseBool(s)
		if e != nil {
			return Options{}, ErrInvalid
		}
		o.Enabled = standard && v
	}
	if s := lookup("WIKI_TESSERACT_MAX_SIZE"); s != "" {
		v, e := strconv.ParseInt(s, 10, 64)
		if e != nil || v <= 0 {
			return Options{}, ErrInvalid
		}
		o.MaxSize = v
	}
	if s := lookup("WIKI_TESSERACT_TIMEOUT"); s != "" {
		v, e := strconv.ParseInt(s, 10, 64)
		if e != nil || v <= 0 || v > 86400000 {
			return Options{}, ErrInvalid
		}
		o.Timeout = time.Duration(v) * time.Millisecond
	}
	if s := lookup("WIKI_TESSERACT_LANGS"); s != "" {
		o.Languages = strings.Split(s, "+")
	}
	return o, nil
}

// Cycle shares the original recognition-then-text-flush ordering. Native Run
// additionally flushes the owner's node queue; compatibility retains its queue.
func Cycle(ctx context.Context, run, flush func(context.Context) error) error {
	if run == nil || flush == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := run(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return flush(ctx)
}
func StatusMessage(name, text string) string {
	return fmt.Sprintf("OCR [%s] [%s]", html.EscapeString(filepath.Base(name)), html.EscapeString(text))
}
