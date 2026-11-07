package pdfdevice

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"os"
	"strings"
	"testing"
	"time"
)

func pipelineFixture(t *testing.T, uris ...string) []byte {
	t.Helper()
	p, e := api.ReadContext(bytes.NewReader(fixture(t)), &pdfmodel.Configuration{Reader15: true, ValidationMode: pdfmodel.ValidationRelaxed, Eol: types.EolLF})
	if e != nil {
		t.Fatal(e)
	}
	if e = api.ValidateContext(p); e != nil {
		t.Fatal(e)
	}
	if _, e = pdfcpu.RemoveAnnotations(p, nil, nil, nil, false); e != nil {
		t.Fatal(e)
	}
	annotations := []pdfmodel.AnnotationRenderer{}
	for _, uri := range uris {
		annotations = append(annotations, pdfmodel.NewLinkAnnotation(*types.RectForWidthAndHeight(10, 40, 80, 20), "", "", "", 0, nil, nil, uri, nil, false, 0, 0))
	}
	if _, e = pdfcpu.AddAnnotationsMap(p, map[int][]pdfmodel.AnnotationRenderer{1: annotations}, false); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e = api.WriteContext(p, &out); e != nil {
		t.Fatal(e)
	}
	return out.Bytes()
}
func TestDevicePipelineInspectAndMissingMaterial(t *testing.T) {
	const origin = "http://127.0.0.1:60111"
	uri := origin + "/assets/chosen.txt"
	input := pipelineFixture(t, uri, "https://example.invalid/assets/private.txt", origin+"9/assets/foreign.txt")
	exe, _ := os.Executable()
	run, e := Bind(exe)
	if e != nil {
		t.Fatal(e)
	}
	out, e := run(context.Background(), Request{Protocol: 1, PDF: input, InspectOrigin: origin})
	if e != nil {
		t.Fatal(e)
	}
	found, e := DecodeInspection(out)
	if e != nil || len(found.URIs) != 1 || found.URIs[0] != uri {
		t.Fatal("inspection scope", found, e)
	}
	request := Request{Protocol: 1, PDF: input, Assets: &Assets{Now: time.Unix(1700000000, 0), Origin: origin, Destinations: []string{"assets/chosen.txt"}, Entries: []Asset{{Path: "assets/chosen.txt", Name: "chosen.txt", Exists: true, ModTime: time.Unix(1700000000, 0)}}, Embed: true}}
	if out, e = run(context.Background(), request); e == nil || out != nil {
		t.Fatal("unloaded material silently became empty attachment", e)
	}
	request.Assets.Entries[0].Loaded = true
	if out, e = run(context.Background(), request); e != nil {
		t.Fatal("explicit empty file rejected", e)
	}
	inspect(t, out, false)
	request.PDF = pipelineFixture(t, origin+"/assets/unselected.txt")
	if out, e = run(context.Background(), request); e == nil || out != nil {
		t.Fatal("unknown local source accepted", e)
	}
}
func TestDevicePipelineProtocolAndInspectionBounds(t *testing.T) {
	base := Request{Protocol: 1, PDF: []byte("%PDF-")}
	for _, mutate := range []func(*Request){func(r *Request) { r.InspectOrigin = "file:///source" }, func(r *Request) { r.InspectOrigin = "http://localhost:1/path" }, func(r *Request) { r.InspectOrigin = "http://localhost:1"; r.Watermark = &Watermark{Mode: "text"} }, func(r *Request) {
		r.Outline = &Outline{Headings: []Heading{{ID: "same", Level: 1}, {ID: "same", Level: 2}}}
	}, func(r *Request) { r.Outline = &Outline{Headings: []Heading{{ID: "id", Level: 7}}} }, func(r *Request) {
		r.Assets = &Assets{Origin: "http://localhost:1", Destinations: []string{"assets/missing"}}
	}, func(r *Request) {
		r.Assets = &Assets{Origin: "http://localhost:1", Entries: []Asset{{Path: "assets/../private", Name: "private"}}}
	}, func(r *Request) {
		r.Assets = &Assets{Origin: "http://localhost:1", Entries: []Asset{{Path: "assets/file", Name: "file", Data: []byte("not marked loaded")}}, Embed: true}
	}, func(r *Request) { r.Fonts = []Font{{Name: "Selected", TTF: []byte("x"), Cache: []byte("y")}} }} {
		r := base
		mutate(&r)
		if Validate(r) == nil {
			t.Fatal("invalid plan accepted")
		}
	}
	if e := Validate(Request{Protocol: 1, PDF: base.PDF, Fonts: []Font{{Name: "旧 Font.1", Cache: []byte("cache")}}}); e != nil {
		t.Fatal("legacy inventory name rejected", e)
	}
	for _, raw := range []string{`{"URIs":[],"source":"/private"}`, `{"URIs":[]} {}`, `{"URIs":["` + strings.Repeat("a", 4097) + `"]}`} {
		if _, e := DecodeInspection([]byte(raw)); e == nil {
			t.Fatal("invalid inspection accepted")
		}
	}
	tooMany := Inspection{URIs: make([]string, 4097)}
	raw, _ := json.Marshal(tooMany)
	if _, e := DecodeInspection(raw); e == nil {
		t.Fatal("unbounded link inspection")
	}
}
