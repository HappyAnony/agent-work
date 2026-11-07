//go:build fts5

package workspace

import (
	"context"
	"errors"
	"github.com/gofrs/flock"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudaccount"
)

func inboxConfiguration(_, page string) (conf.AppData, error) {
	d := conf.AppData{Editor: conf.NewEditor(), Export: conf.NewExport(), FileTree: conf.NewFileTree(), Search: conf.NewSearch()}
	d.Editor.Markdown.InlineMath = page == pageA
	return d, nil
}
func nativeInboxResponse(page string, detail bool) cloudaccount.Response {
	item := map[string]any{"oId": "1760000000000", "shorthandDesc": "fixture", "shorthandContent": "owned " + page + " $a+b$"}
	var data any = item
	if !detail {
		data = map[string]any{"shorthands": []any{item}, "pagination": map[string]any{"paginationRecordCount": float64(1), "paginationPageCount": float64(1)}}
	}
	return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0), "data": data}}
}

func TestNativeInboxActualOwnersParserIsolationAndNoSourceWrite(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	records := map[string]map[string]any{}
	m := open(t, root, Options{Configuration: inboxConfiguration, CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
		o := businessOptions(t, page)
		load := o.Load
		o.Load = func(ctx context.Context) (cloudaccount.State, error) {
			v, e := load(ctx)
			v.User.UserId = "same-account-id"
			return v, e
		}
		calls := 0
		o.InboxRequest = func(ctx context.Context, region int, route string, h map[string]string, b map[string]any) (cloudaccount.Response, error) {
			calls++
			want := 0
			if page == pageB {
				want = 1
			}
			if region != want || h["Cookie"] != "symphony=fixture-"+page {
				t.Error("borrowed inbox identity")
			}
			if strings.HasSuffix(route, "removeCloudShorthands") {
				ids, ok := b["ids"].([]string)
				if !ok || len(ids) != 1 || ids[0] != "1760000000000" {
					t.Error("wrong deletion scope")
				}
				records[page] = map[string]any{"region": region, "calls": calls}
				return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0)}}, nil
			}
			if b != nil {
				t.Error("added body to inbox read")
			}
			return nativeInboxResponse(page, strings.Contains(route, "getCloudShorthand?")), nil
		}
		return o, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		list, e := l.NativeCloudGetShorthands(context.Background(), 1)
		if e != nil {
			t.Fatal(e)
		}
		item := list["data"].(map[string]any)["shorthands"].([]any)[0].(map[string]any)
		detail, e := l.NativeCloudGetShorthand(context.Background(), "1760000000000")
		if e != nil {
			t.Fatal(e)
		}
		wantPage := pageA
		if l == b {
			wantPage = pageB
		}
		if !strings.Contains(detail["shorthandMd"].(string), wantPage) || detail["shorthandMd"] != item["shorthandMd"] || strings.Contains(detail["shorthandContent"].(string), "math") != (l == a) {
			t.Fatal("parser/content leaked", detail)
		}
		if e := l.NativeCloudRemoveShorthands(context.Background(), []string{"1760000000000"}); e != nil {
			t.Fatal(e)
		}
	}
	a.Release()
	b.Release()
	if e := m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(records) != 2 || digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("inbox source write/task leak")
	}
	recordCloudBusinessEvidence(t, "inbox-contexts.json", map[string]any{"owners": records, "sameIDOwnerIsolation": true, "parserIsolationVerified": true, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks, "rawCredentialsRecorded": false})
}

func TestNativeInboxMissingCapabilityAndAllOperationsDrain(t *testing.T) {
	unbound := open(t, fixture(t), Options{})
	u := getDocLease(t, unbound, pageA)
	if _, e := u.NativeCloudGetShorthands(context.Background(), 1); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	if _, e := u.NativeCloudGetShorthand(context.Background(), "fixture"); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	if e := u.NativeCloudRemoveShorthands(context.Background(), []string{"fixture"}); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	u.Release()
	for _, op := range []string{"list", "detail", "remove"} {
		t.Run(op, func(t *testing.T) {
			root := fixture(t)
			before := digest(t, root)
			entered, exited := make(chan struct{}), make(chan struct{})
			m := open(t, root, Options{Configuration: inboxConfiguration, CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
				o := businessOptions(t, page)
				o.InboxRequest = func(ctx context.Context, _ int, _ string, _ map[string]string, _ map[string]any) (cloudaccount.Response, error) {
					close(entered)
					<-ctx.Done()
					close(exited)
					return cloudaccount.Response{}, ctx.Err()
				}
				return o, nil
			}})
			l := getDocLease(t, m, pageA)
			done := make(chan error, 1)
			go func() {
				var e error
				switch op {
				case "list":
					_, e = l.NativeCloudGetShorthands(context.Background(), 1)
				case "detail":
					_, e = l.NativeCloudGetShorthand(context.Background(), "fixture")
				default:
					e = l.NativeCloudRemoveShorthands(context.Background(), []string{"fixture"})
				}
				done <- e
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("inbox never started")
			}
			lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
			ok, e := lock.TryLock()
			if e != nil || !ok {
				t.Fatal("network retained source lock", e)
			}
			if e = lock.Unlock(); e != nil {
				t.Fatal(e)
			}
			l.Release()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if e = m.Drain(ctx); e != nil {
				t.Fatal(e)
			}
			select {
			case <-exited:
			default:
				t.Fatal("drain failed to join")
			}
			select {
			case e = <-done:
				if !errors.Is(e, context.Canceled) {
					t.Fatal(e)
				}
			case <-ctx.Done():
				t.Fatal("caller blocked")
			}
			if digest(t, root) != before || m.Stats().Tasks != 0 {
				t.Fatal("source changed/task leaked")
			}
			recordCloudBusinessEvidence(t, "inbox-lifecycle-"+op+".json", map[string]any{"operation": op, "exclusiveSourceLockAvailableDuringNetwork": ok, "drainCancelledAndJoinedTransport": true, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks})
		})
	}
}
