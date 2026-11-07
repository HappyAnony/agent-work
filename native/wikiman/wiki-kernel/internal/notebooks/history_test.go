package notebooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeletedHistoryOriginalMetadataOrderAndNoPartialOnCorruption(t *testing.T) {
	dir, root := fixture(t)
	put(t, dir, "2026-11-03-120000-delete/"+good+"/.siyuan/conf.json", `{"name":"Earlier"}`)
	put(t, dir, "2026-11-04-120000-delete/"+good+"/.siyuan/conf.json", `{"name":"Later"}`)
	put(t, dir, "2026-11-04-120000-update/"+good+"/.siyuan/conf.json", `{"name":"Ignored"}`)
	out, e := Deleted(context.Background(), root)
	if e != nil || len(out) != 2 || out[0].Name != "Later" || out[0].Created != "2026-11-04 12:00:00" || out[0].Path != "2026-11-04-120000-delete/"+good {
		t.Fatal(out, e)
	}
	put(t, dir, "2026-11-04-120000-delete/"+good+"/.siyuan/conf.json", `corrupt`)
	if out, e = Deleted(context.Background(), root); e == nil || out != nil {
		t.Fatal("partial result escaped", out, e)
	}
	put(t, dir, "2026-11-04-120000-delete/"+good+"/.siyuan/conf.json", `{"name":"Later"}`)
	if e = os.Symlink(t.TempDir(), filepath.Join(dir, "2026-11-05-120000-delete")); e != nil {
		t.Fatal(e)
	}
	if out, e = Deleted(context.Background(), root); e == nil || out != nil {
		t.Fatal("linked history accepted", out, e)
	}
}
