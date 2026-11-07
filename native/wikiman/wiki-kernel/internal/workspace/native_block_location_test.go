//go:build fts5

package workspace

import (
	"context"
	"strings"
	"testing"
)

func TestNativeBlockLocationActualOwnerProjection(t *testing.T) {
	ctx := context.Background()
	root := relationsSource(t)
	before := digest(t, root)
	m := open(t, root, positionOptions(root))
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		out, e := l.NativeBlockLocation(ctx, blockID)
		if e != nil || out["box"] != boxID || out["rootID"] != docID || !strings.Contains(out["rootTitle"], page) || out["rootChildID"] != docID {
			t.Fatal(out, e)
		}
		out["rootTitle"] = "changed"
		again, e := l.NativeBlockLocation(ctx, blockID)
		if e != nil || again["rootTitle"] == "changed" {
			t.Fatal(again, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("location changed source")
	}
}
