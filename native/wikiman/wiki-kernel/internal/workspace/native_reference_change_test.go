//go:build fts5

package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNativeReferenceChangeActualOwnersCandidatesAndNoPublication(t *testing.T) {
	ctx := context.Background()
	root := positionSource(t)
	before := digest(t, root)
	m := open(t, root, lifecycleOptions(root))
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		queueGetDoc(t, l)
		if e := l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		swap, e := l.PlanNativeReferenceChange(ctx, NativeReferenceChangeRequest{Mode: "swap", FromID: positionRef, ToID: getDocDefinition})
		if e != nil || len(swap.Trees) == 0 || len(swap.SourceHashes) != 2 {
			t.Fatal(page, swap, e)
		}
		plan, e := l.PlanNativeReferenceChange(ctx, NativeReferenceChangeRequest{Mode: "transfer", FromID: blockID, ToID: getDocDefinition})
		if e != nil || len(plan.Trees) != 1 || len(plan.SourceHashes) != 2 {
			t.Fatal(page, plan, e)
		}
		found := false
		for _, tree := range plan.Trees {
			if strings.Contains(tree.Root.Text(), page) {
				found = true
			}
		}
		if !found {
			t.Fatal("candidate borrowed other owner")
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if p, e := l.PlanNativeReferenceChange(canceled, NativeReferenceChangeRequest{Mode: "swap", FromID: positionRef, ToID: getDocDefinition}); !errors.Is(e, context.Canceled) || p.Trees != nil {
			t.Fatal(p, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("candidate published source")
	}
}
