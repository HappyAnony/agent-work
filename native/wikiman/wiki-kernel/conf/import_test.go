package conf

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestConfigurationImportWhitelistReplacementAndIdentity(t *testing.T) {
	before := settingFixture()
	before.Tag = NewTag()
	before.Graph = NewGraph()
	before.Bazaar = NewBazaar()
	keymap := Keymap{}
	before.Keymap = &keymap
	before.System = NewSystemForDevice("owner", "device", "version")
	before.System.DataDir = "/owner/data"
	before.Lang = "en_US"
	before.UserData = "private"
	before.Search.Limit = 90
	before.Editor.ReadOnly = true
	before.Export.PandocBin = "owned-tool"
	before.System.DisabledFeatures = []string{"owned-limit"}
	raw := []byte(`{"search":{"limit":2},"system":{"id":"foreign","dataDir":"/foreign","networkProxy":{"host":"proxy"},"networkServe":true},"userData":"foreign","lang":"foreign","readOnly":false}`)
	plan, e := before.PrepareImport(context.Background(), raw)
	if e != nil {
		t.Fatal(e)
	}
	if plan.Data.Search.Limit != 2 || plan.Data.Search.VirtualRefName || plan.Data.UserData != "private" || plan.Data.Lang != "en_US" || plan.Data.System.ID != "owner" || plan.Data.System.DataDir != "/owner/data" || !plan.Data.System.NetworkServe || plan.Data.Editor.FontSize != before.Editor.FontSize || len(plan.Effects) != 0 {
		t.Fatal(plan)
	}
	plan.Data.System.NetworkProxy.Host = "poison"
	if before.System.NetworkProxy.Host != "" {
		t.Fatal("borrowed system")
	}
	patches, e := plan.PreferencePatches(before)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range patches {
		if p.Set["userData"] != nil || p.Set["system.dataDir"] != nil {
			t.Fatal("authority imported", patches)
		}
	}
	full, _ := json.Marshal(before)
	p, e := before.PrepareImport(context.Background(), full)
	if e != nil || p.Data.Editor.FontSize != before.Editor.FontSize {
		t.Fatal(e)
	}
	protected, e := before.PrepareImport(context.Background(), []byte(`{"editor":{"markdown":{},"readOnly":false},"export":{"pandocBin":"/foreign/tool"},"system":{"networkProxy":{},"disabledFeatures":[]}}`))
	if e != nil || !protected.Data.Editor.ReadOnly || protected.Data.Export.PandocBin != "owned-tool" || len(protected.Data.System.DisabledFeatures) != 1 {
		t.Fatal(protected, e)
	}
}
func TestConfigurationImportRejectsBrokenGroupsAndCancellation(t *testing.T) {
	before := settingFixture()
	for _, raw := range []string{`null`, `[]`, `{}`, `{"userData":"foreign"}`, `{"editor":null}`, `{"editor":{"markdown":null}}`, `{"ai":{}}`, `{"system":{}}`, `{"search":{"limit":"bad"}}`, `{"search":{},"search":{}}`} {
		if _, e := before.PrepareImport(context.Background(), []byte(raw)); e == nil {
			t.Fatal(raw)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := before.PrepareImport(ctx, []byte(`{"search":{}}`)); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := before.PrepareImport(context.Background(), []byte(`{"uiLayout":null}`)); e != nil {
		t.Fatal(e)
	}
}
func TestConfigurationImportJSONZipAndUnsafeArchives(t *testing.T) {
	ctx := context.Background()
	payload := []byte(`{"search":{"limit":42}}`)
	raw, e := ReadImport(ctx, "../fixture.JSON", bytes.NewReader(payload))
	if e != nil || !bytes.Equal(raw, payload) {
		t.Fatal(e)
	}
	archive := func(names ...string) []byte {
		var out bytes.Buffer
		w := zip.NewWriter(&out)
		for _, name := range names {
			f, e := w.Create(name)
			if e != nil {
				t.Fatal(e)
			}
			_, _ = f.Write(payload)
		}
		if e := w.Close(); e != nil {
			t.Fatal(e)
		}
		return out.Bytes()
	}
	raw, e = ReadImport(ctx, "fixture.ZIP", bytes.NewReader(archive("conf.json")))
	if e != nil || !bytes.Equal(raw, payload) {
		t.Fatal(e)
	}
	for _, blob := range [][]byte{archive("../conf.json"), archive("/conf.json"), archive("a\\conf.json"), archive("conf.json", "other.json"), []byte("bad")} {
		if _, e = ReadImport(ctx, "f.zip", bytes.NewReader(blob)); !errors.Is(e, ErrImport) {
			t.Fatal(e)
		}
	}
	if _, e = ReadImport(ctx, "f.json", bytes.NewReader(bytes.Repeat([]byte(" "), MaxImportJSON+1))); !errors.Is(e, ErrImport) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = ReadImport(ctx, "f.json", bytes.NewReader(payload)); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
