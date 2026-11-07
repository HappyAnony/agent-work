//go:build fts5

package workspace

import (
	"context"
	"strings"
	"testing"
)

func TestNativeFormatCandidateKeepsOwnerIdentityAndSource(t *testing.T) {
	root := infoSource(t)
	m := open(t, root, lifecycleOptions(root))
	before := digest(t, root)
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		p, e := l.PlanNativeFormat(context.Background(), docID)
		if e != nil || p.Tree.ID != docID || p.Tree.Box != boxID || !strings.Contains(p.Tree.Root.IALAttr("title"), page) || len(p.SourceHashes) == 0 {
			t.Fatal(p, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("format candidate published source")
	}
}
