package ocr

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExplicitDeviceBootstrapCycleAndOriginalStatus(t *testing.T) {
	env := map[string]string{"WIKI_TESSERACT_ENABLED": "false", "WIKI_TESSERACT_MAX_SIZE": "123", "WIKI_TESSERACT_TIMEOUT": "42", "WIKI_TESSERACT_LANGS": "eng+chi_sim"}
	o, e := DeviceOptions(func(k string) string { return env[k] }, true)
	if e != nil || o.Enabled || o.MaxSize != 123 || o.Timeout != 42*time.Millisecond || len(o.Languages) != 2 {
		t.Fatal(o, e)
	}
	env["WIKI_TESSERACT_TIMEOUT"] = "-1"
	if _, e = DeviceOptions(func(k string) string { return env[k] }, true); e == nil {
		t.Fatal("negative timeout")
	}
	var order []string
	e = Cycle(context.Background(), func(context.Context) error { order = append(order, "run"); return nil }, func(context.Context) error { order = append(order, "flush"); return nil })
	if e != nil || strings.Join(order, ",") != "run,flush" {
		t.Fatal(order, e)
	}
	want := errors.New("failure")
	e = Cycle(context.Background(), func(context.Context) error { return want }, func(context.Context) error { t.Fatal("flushed failed run"); return nil })
	if !errors.Is(e, want) {
		t.Fatal(e)
	}
	if got := StatusMessage("assets/a<b.png", "<script>&"); got != "OCR [a&lt;b.png] [&lt;script&gt;&amp;]" {
		t.Fatal(got)
	}
}
