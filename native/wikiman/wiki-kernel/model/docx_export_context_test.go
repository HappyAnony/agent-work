//go:build fts5

package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func docxFixture(t *testing.T) (*markdownExportContext, string, map[string][]byte) {
	t.Helper()
	m, id, before := fileContextFixture(t)
	oldWork, oldAppearance := util.WorkingDir, util.AppearancePath
	util.WorkingDir = m.htmlResources.work.path
	util.AppearancePath = filepath.Dir(filepath.Dir(m.htmlResources.theme.path))
	t.Cleanup(func() { util.WorkingDir, util.AppearancePath = oldWork, oldAppearance })
	Conf.Appearance = conf.NewAppearance()
	Conf.Appearance.ThemeLight = "custom"
	Conf.Appearance.Icon = "custom"
	Conf.Export.AddTitle = false
	return m, id, before
}
func TestDocxExportOwnedConversionTemplateAndDestination(t *testing.T) {
	m, id, before := docxFixture(t)
	template := archiveWrite(t, t.TempDir(), "reference.docx", []byte("reference bytes"))
	bin := pandocFixture(t, `[ "$1" = "--version" ] && { echo 'pandoc 3'; exit 0; }
input="$1"
shift
while [ "$#" -gt 0 ]; do
 case "$1" in
 --from) [ "$2" = "html+tex_math_dollars" ] || exit 21; shift 2;;
 --to) [ "$2" = "docx" ] || exit 22; shift 2;;
 --resource-path) [ -f "$2/assets/same.pdf" ] || exit 23; shift 2;;
 -o) output="$2"; shift 2;;
 --reference-doc) [ "$(cat "$2")" = 'reference bytes' ] || exit 24; shift 2;;
 -s) shift;;
 *) exit 25;;
 esac
done
cp "$input" "$output"
`)
	Conf.Export.PandocBin = bin
	Conf.Export.DocxTemplate = template
	folder := t.TempDir()
	out, e := ExportDocxContext(context.Background(), id, folder, false, false)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(out)
	if e != nil || !strings.Contains(string(raw), "File heading") {
		t.Fatal(string(raw), e)
	}
	if raw, e = os.ReadFile(filepath.Join(folder, "assets/same.pdf")); e != nil || len(raw) == 0 {
		t.Fatal(e)
	}
	second, e := ExportDocxContext(context.Background(), id, folder, true, false)
	if e != nil || second == out {
		t.Fatal(second, e)
	}
	if _, e = ExportDocxContext(context.Background(), id, util.DataDir, true, false); e == nil {
		t.Fatal("source destination admitted")
	}
	Conf.Export.DocxTemplate = filepath.Join(t.TempDir(), "missing.docx")
	if out, e = ExportDocxContext(context.Background(), id, t.TempDir(), true, false); e == nil || out != "" {
		t.Fatal(out, e)
	}
	assertFileSourceUnchanged(t, before)
	assertNoFileWork(t, m)
}

func TestDocxExportFallbackCapturedAndCancellation(t *testing.T) {
	m, id, before := docxFixture(t)
	oldBin := util.PandocBinPath
	defer func() { util.PandocBinPath = oldBin }()
	bin := pandocFixture(t, `[ "$1" = "--version" ] && { echo 'pandoc 3'; exit 0; }
while [ "$#" -gt 0 ]; do
 if [ "$1" = '-o' ]; then printf converted > "$2"; exit 0; fi
 shift
done
exit 1
`)
	Conf.Export.PandocBin = "/missing/pandoc"
	Conf.Export.DocxTemplate = ""
	util.PandocBinPath = bin
	out, e := ExportDocxContext(context.Background(), id, t.TempDir(), true, false)
	if e != nil {
		t.Fatal(e)
	}
	if raw, e := os.ReadFile(out); e != nil || string(raw) != "converted" {
		t.Fatal(e)
	}
	if Conf.Export.PandocBin != bin {
		t.Fatal("fallback not captured/persisted")
	}
	slow := pandocFixture(t, `[ "$1" = "--version" ] && { echo 'pandoc 3'; exit 0; }
exec sleep 30
`)
	Conf.Export.PandocBin = slow
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	out, e = ExportDocxContext(ctx, id, t.TempDir(), true, false)
	if !errors.Is(e, context.DeadlineExceeded) || out != "" || time.Since(start) > 3*time.Second {
		t.Fatal(out, e, time.Since(start))
	}
	assertFileSourceUnchanged(t, before)
	assertNoFileWork(t, m)
}
