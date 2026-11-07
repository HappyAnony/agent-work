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
	"wiki-kernel/internal/aiflow"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/indexdb"
)

type nativeAIClient struct {
	chat  func(context.Context, string, []string) (aiflow.Part, error)
	close func()
}

func (c *nativeAIClient) Chat(ctx context.Context, m string, h []string) (aiflow.Part, error) {
	return c.chat(ctx, m, h)
}
func (c *nativeAIClient) Close() {
	if c.close != nil {
		c.close()
	}
}
func nativeAIConfiguration(_, page string) (conf.AppData, error) {
	v := conf.AppData{Editor: conf.NewEditor(), Export: conf.NewExport(), AI: conf.NewAI(), FileTree: conf.NewFileTree(), Search: conf.NewSearch()}
	v.AI.OpenAI.APIKey = "fixture-" + page
	v.AI.OpenAI.APIModel = "model-" + page
	v.AI.OpenAI.APIBaseURL = "http://fixture.invalid/" + page
	v.AI.OpenAI.APIMaxContexts = 2
	return v, nil
}
func TestNativeAIActualPageConfigurationSourceAndHistoryIsolation(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	histories := map[string][]int{}
	closed := map[string]int{}
	sourceVerified := map[string]bool{}
	m := open(t, root, Options{Configuration: nativeAIConfiguration, AIOptions: func(_, page string) (aiflow.Options, error) {
		return aiflow.Options{Event: func(context.Context, aiflow.Event) {}, Open: func(v conf.OpenAI) (aiflow.Client, error) {
			if v.APIKey != "fixture-"+page || v.APIModel != "model-"+page || !strings.HasSuffix(v.APIBaseURL, page) {
				t.Error("foreign private configuration")
			}
			return &nativeAIClient{chat: func(ctx context.Context, msg string, h []string) (aiflow.Part, error) {
				histories[page] = append(histories[page], len(h))
				if strings.HasPrefix(msg, "summarize:") {
					want := "Synthetic knowledge"
					if page == pageB {
						want = "Workspace B knowledge"
					}
					sourceVerified[page] = strings.Contains(msg, want)
					if !sourceVerified[page] || len(h) != 0 {
						t.Error("source/action history crossed pages")
					}
				}
				return aiflow.Part{Text: "reply-" + page, Stop: true}, ctx.Err()
			}, close: func() { closed[page]++ }}, nil
		}}, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		page := pageA
		if l == b {
			page = pageB
		}
		if text, e := l.NativeAIChat(context.Background(), "one"); e != nil || text != "reply-"+page {
			t.Fatal(text, e)
		}
		if _, e := l.NativeAIChat(context.Background(), "two"); e != nil {
			t.Fatal(e)
		}
		if _, e := l.NativeAIAction(context.Background(), []string{docID, blockID, blockID}, "summarize"); e != nil {
			t.Fatal(e)
		}
		if _, e := l.NativeAIAction(context.Background(), nil, "Clear context"); e != nil {
			t.Fatal(e)
		}
		if _, e := l.NativeAIChat(context.Background(), "after clear"); e != nil {
			t.Fatal(e)
		}
	}
	for _, page := range []string{pageA, pageB} {
		h := histories[page]
		if len(h) != 4 || h[0] != 0 || h[1] != 2 || h[2] != 0 || h[3] != 0 || closed[page] != 4 || !sourceVerified[page] {
			t.Fatal(h, closed, sourceVerified)
		}
	}
	a.Release()
	b.Release()
	if e := m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("AI wrote source or leaked tasks")
	}
	recordCloudBusinessEvidence(t, "ai-contexts.json", map[string]any{"historyMessageCounts": histories, "closedClients": closed, "ownerConfigurationAndSourceVerified": true, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks, "rawPromptOrCredentialsRecorded": false})
}
func TestNativeAIDirectAndCloudRequestDrainReleasesSourceAndResourceGates(t *testing.T) {
	uManager := open(t, fixture(t), Options{})
	u := getDocLease(t, uManager, pageA)
	if _, e := u.NativeAIChat(context.Background(), "fixture"); !errors.Is(e, aiflow.ErrDependency) {
		t.Fatal(e)
	}
	if _, e := u.NativeCloudChatGPT(context.Background(), "fixture", nil); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	u.Release()
	bad := open(t, fixture(t), Options{AIOptions: func(string, string) (aiflow.Options, error) { return aiflow.Options{}, nil }})
	if l, e := bad.Acquire(context.Background(), storeID, pageA); !errors.Is(e, aiflow.ErrDependency) || l != nil || bad.Stats().Workspaces != 0 {
		t.Fatal("failed initialization published owner", l, e)
	}
	for _, mode := range []string{"direct", "cloud"} {
		t.Run(mode, func(t *testing.T) {
			root := fixture(t)
			before := digest(t, root)
			entered, exited := make(chan struct{}), make(chan struct{})
			clientClosed := false
			clear := false
			block := func(c context.Context) error { close(entered); <-c.Done(); close(exited); return c.Err() }
			opts := Options{Configuration: nativeAIConfiguration}
			if mode == "direct" {
				opts.AIOptions = func(string, string) (aiflow.Options, error) {
					return aiflow.Options{Event: func(c context.Context, e aiflow.Event) {
						if e == aiflow.Clear {
							if c.Err() != nil {
								t.Error("cleanup canceled")
							}
							clear = true
						}
					}, Open: func(conf.OpenAI) (aiflow.Client, error) {
						return &nativeAIClient{chat: func(c context.Context, _ string, _ []string) (aiflow.Part, error) { return aiflow.Part{}, block(c) }, close: func() { clientClosed = true }}, nil
					}}, nil
				}
			} else {
				opts.CloudAccountOptions = func(_, page string) (cloudaccount.Options, error) {
					o := businessOptions(t, page)
					o.AIRequest = func(c context.Context, _ int, _ string, _ map[string]string, _ map[string]any) (cloudaccount.Response, error) {
						return cloudaccount.Response{}, block(c)
					}
					return o, nil
				}
			}
			m := open(t, root, opts)
			l := getDocLease(t, m, pageA)
			done := make(chan error, 1)
			go func() {
				var e error
				if mode == "direct" {
					_, e = l.NativeAIAction(context.Background(), []string{blockID}, "rewrite")
				} else {
					_, e = l.NativeCloudChatGPT(context.Background(), "fixture", nil)
				}
				done <- e
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not enter")
			}
			lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
			ok, e := lock.TryLock()
			if e != nil || !ok {
				t.Fatal("network retained source lock", e)
			}
			lock.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if e = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { return nil }); e != nil {
				t.Fatal("network retained resource gate", e)
			}
			l.Release()
			if e = m.Drain(ctx); e != nil {
				t.Fatal(e)
			}
			select {
			case <-exited:
			default:
				t.Fatal("request not joined")
			}
			select {
			case e = <-done:
				if !errors.Is(e, context.Canceled) {
					t.Fatal(e)
				}
			case <-ctx.Done():
				t.Fatal("AI caller stuck")
			}
			if mode == "direct" && (!clientClosed || !clear) {
				t.Fatal("client/progress not closed")
			}
			if digest(t, root) != before || m.Stats().Tasks != 0 {
				t.Fatal("source changed or task leaked")
			}
			recordCloudBusinessEvidence(t, "ai-lifecycle-"+mode+".json", map[string]any{"mode": mode, "exclusiveSourceLockAvailable": ok, "resourceGateAvailable": true, "requestCancelledAndJoined": true, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks})
		})
	}
}
