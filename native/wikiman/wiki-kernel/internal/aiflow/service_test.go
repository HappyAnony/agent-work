package aiflow

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/nativeparser"
)

type fakeClient struct {
	chat  func(context.Context, string, []string) (Part, error)
	close func()
}

func (f *fakeClient) Chat(c context.Context, m string, h []string) (Part, error) {
	return f.chat(c, m, h)
}
func (f *fakeClient) Close() {
	if f.close != nil {
		f.close()
	}
}
func settings() Settings {
	v := *conf.NewAI().OpenAI
	v.APIKey = "synthetic"
	m := nativeparser.Defaults()
	return Settings{API: v, Markdown: &m}
}
func open(t *testing.T, o Options) *Service {
	t.Helper()
	s, e := New(o)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(context.Background()); e != nil {
			t.Error(e)
		}
	})
	return s
}
func TestAIOriginalHistoryClearActionAndConfigurationIdentity(t *testing.T) {
	cfg := settings()
	cfg.API.APIMaxContexts = 2
	var histories [][]string
	var prompts []string
	closed := 0
	events := []Event{}
	s := open(t, Options{Load: func(context.Context) (Settings, error) { return cfg, nil }, Event: func(_ context.Context, e Event) { events = append(events, e) }, Open: func(v conf.OpenAI) (Client, error) {
		return &fakeClient{chat: func(_ context.Context, m string, h []string) (Part, error) {
			prompts = append(prompts, m)
			histories = append(histories, h)
			return Part{" reply ", true}, nil
		}, close: func() { closed++ }}, nil
	}})
	if v, e := s.Chat(context.Background(), "one"); e != nil || v != "reply" {
		t.Fatal(v, e)
	}
	s.Chat(context.Background(), "two")
	if !reflect.DeepEqual(histories[1], []string{"one", "reply"}) {
		t.Fatal(histories)
	}
	before := append([]string(nil), s.history...)
	extract := func(_ context.Context, ids []string, _ Settings) (string, error) {
		if !reflect.DeepEqual(ids, []string{"a", "a"}) {
			t.Fatal(ids)
		}
		return "source\n\n", nil
	}
	s.Action(context.Background(), []string{"a", "a"}, " summarize ", extract)
	if prompts[2] != "summarize:\n\nsource\n\n" || len(histories[2]) != 0 || !reflect.DeepEqual(s.history, before) {
		t.Fatal(prompts, histories, s.history)
	}
	count := len(prompts)
	s.Action(context.Background(), nil, "Clear context", func(context.Context, []string, Settings) (string, error) {
		t.Fatal("clear exported source")
		return "", nil
	})
	if len(s.history) != 0 || len(prompts) != count {
		t.Fatal("clear contacted provider")
	}
	s.Chat(context.Background(), "new")
	cfg.API.APIBaseURL = "http://different.invalid/v1"
	s.Chat(context.Background(), "endpoint change")
	if len(histories[len(histories)-1]) != 0 {
		t.Fatal("history crossed endpoint identity")
	}
	s.Chat(context.Background(), " Clear context ")
	if len(s.history) != 0 {
		t.Fatal("trimmed chat clear changed")
	}
	if closed != len(prompts) || events[len(events)-1] != Clear {
		t.Fatal(closed, events)
	}
}
func TestAIContinuationUsesOriginalInputsAndFailureDoesNotCacheTurn(t *testing.T) {
	cfg := settings()
	cfg.API.APIMaxContexts = 2
	round := 0
	fail := false
	var history [][]string
	var messages []string
	s := open(t, Options{Load: func(context.Context) (Settings, error) { return cfg, nil }, Event: func(context.Context, Event) {}, Open: func(conf.OpenAI) (Client, error) {
		return &fakeClient{chat: func(_ context.Context, m string, h []string) (Part, error) {
			round++
			history = append(history, h)
			messages = append(messages, m)
			if fail {
				return Part{" partial", false}, errors.New("sensitive provider body")
			}
			return Part{" x", false}, nil
		}}, nil
	}})
	v, e := s.Chat(context.Background(), "prompt")
	if e != nil || v != "x x" || round != 2 || messages[0] != messages[1] || !reflect.DeepEqual(history[0], history[1]) {
		t.Fatal(v, e, history, messages)
	}
	previous := append([]string(nil), s.history...)
	fail = true
	v, e = s.Chat(context.Background(), "failed turn")
	if !errors.Is(e, ErrProvider) || v != "partial" || !reflect.DeepEqual(s.history, previous) {
		t.Fatal(v, e, s.history)
	}
	fail = false
	for i := 0; i < 50; i++ {
		if _, e := s.Chat(context.Background(), "bounded"); e != nil {
			t.Fatal(e)
		}
	}
	if len(s.history) != 64 {
		t.Fatal("unbounded history", len(s.history))
	}
}
func TestAIClosedOwnerCancelsRequestJoinsClientAndClearsProgress(t *testing.T) {
	entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	clientClosed, cleared := false, false
	s := open(t, Options{Load: func(context.Context) (Settings, error) { return settings(), nil }, Open: func(conf.OpenAI) (Client, error) {
		return &fakeClient{chat: func(c context.Context, _ string, _ []string) (Part, error) {
			close(entered)
			<-release
			close(exited)
			return Part{}, c.Err()
		}, close: func() { clientClosed = true }}, nil
	}, Event: func(c context.Context, e Event) {
		if e == Clear {
			if c.Err() != nil {
				t.Error("cleanup inherited cancellation")
			}
			cleared = true
		}
	}})
	done := make(chan error, 1)
	go func() { _, e := s.Chat(context.Background(), "fixture"); done <- e }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e := s.Close(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	close(release)
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("request not joined")
	}
	if !clientClosed || !cleared || len(s.history) != 0 {
		t.Fatal("resources/history leaked")
	}
}
func TestAIMissingKeyInvalidSettingsAndTypedDeadline(t *testing.T) {
	cfg := settings()
	cfg.API.APIKey = ""
	opened := 0
	missing := 0
	s := open(t, Options{Load: func(context.Context) (Settings, error) { return cfg, nil }, Open: func(conf.OpenAI) (Client, error) {
		opened++
		return &fakeClient{chat: func(context.Context, string, []string) (Part, error) { return Part{}, context.DeadlineExceeded }}, nil
	}, Event: func(_ context.Context, e Event) {
		if e == MissingKey {
			missing++
		}
	}})
	if v, e := s.Chat(context.Background(), "fixture"); v != "" || e != nil || opened != 0 || missing != 1 {
		t.Fatal(v, e, opened, missing)
	}
	cfg.API.APIKey = "synthetic"
	cfg.API.APIMaxContexts = 0
	if _, e := s.Chat(context.Background(), "fixture"); !errors.Is(e, ErrConfig) || opened != 0 {
		t.Fatal(e)
	}
	cfg.API.APIMaxContexts = 7
	if _, e := s.Chat(context.Background(), "fixture"); !errors.Is(e, context.DeadlineExceeded) || len(s.history) != 0 {
		t.Fatal(e)
	}
}
func TestAIBlocksOriginalDocumentChildrenRepeatedIDsAndMarkdown(t *testing.T) {
	p := nativeparser.NewLute(nativeparser.Defaults())
	tree := parse.Parse("", []byte("# title\n\n**body**\n"), p.ParseOptions)
	tree.Root.ID = "doc"
	a := tree.Root.FirstChild
	b := a.Next
	a.ID = "a"
	b.ID = "b"
	trees := map[string]*parse.Tree{"doc": tree, "b": tree}
	v, e := Blocks(context.Background(), []string{"doc", "missing", "b", "b"}, trees, nativeparser.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	var want strings.Builder
	var originalNodes []*ast.Node
	for n := tree.Root.FirstChild; n != nil; n = n.Next {
		originalNodes = append(originalNodes, n)
	}
	originalNodes = append(originalNodes, b, b)
	for _, n := range originalNodes {
		md, e := lute.ProtyleExportMdNodeSync(n, p.ParseOptions, p.RenderOptions)
		if e != nil {
			t.Fatal(e)
		}
		want.WriteString(md)
		want.WriteString("\n\n")
	}
	if v != want.String() {
		t.Fatal(v, want.String())
	}
	c, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Blocks(c, []string{"doc"}, trees, nativeparser.Defaults()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
