package server

import (
	"context"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wiki-kernel/conf"
	"wiki-kernel/internal/aiflow"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func privateDevicePath(rootPath, filePath string) (string, error) {
	root, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(filePath))
	if err != nil {
		return "", err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	name := filepath.Join(parent, filepath.Base(filePath))
	relative, err := filepath.Rel(root, name)
	if err != nil || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", wikiops.ErrArgument
	}
	return name, nil
}

func privateDeviceFile(rootPath, filePath string) (string, error) {
	name, err := privateDevicePath(rootPath, filePath)
	if err != nil {
		return "", err
	}
	return tokenFile(name)
}

func deviceAI(config Config) (func(string, string) (aiflow.Options, error), error) {
	if config.AIFile == "" {
		return nil, nil
	}
	raw, err := privateDeviceFile(config.Root, config.AIFile)
	if err != nil {
		return nil, wikiops.ErrArgument
	}
	q := struct {
		StoreID string      `json:"storeId"`
		OpenAI  conf.OpenAI `json:"openAI"`
	}{OpenAI: *conf.NewAI().OpenAI}
	q.OpenAI.APITimeout = 20
	if err = wikiops.Decode(strings.NewReader(raw), &q); err != nil {
		return nil, wikiops.ErrArgument
	}
	if q.StoreID == "" {
		return nil, wikiops.ErrArgument
	}
	if q.StoreID != config.StoreID {
		return nil, nil
	}
	v := q.OpenAI
	u, err := url.Parse(v.APIBaseURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback()) ||
		v.APIProxy != "" || v.APIKey == "" || v.APIModel == "" ||
		v.APIProvider != "OpenAI" && v.APIProvider != "Azure" ||
		v.APITimeout < 1 || v.APITimeout > 25 || v.APIMaxContexts < 1 || v.APIMaxContexts > 16 ||
		v.APIMaxTokens < 0 || v.APITemperature <= 0 || v.APITemperature > 2 {
		return nil, wikiops.ErrArgument
	}
	type conversationEntry struct {
		state *aiflow.Conversation
		last  time.Time
	}
	conversations := make(map[string]conversationEntry)
	var mu sync.Mutex
	return func(store, page string) (aiflow.Options, error) {
		if store != q.StoreID {
			return aiflow.Options{}, workspace.ErrIdentity
		}
		mu.Lock()
		entry := conversations[page]
		if entry.state == nil {
			if len(conversations) == 32 {
				var oldest string
				for id, item := range conversations {
					if oldest == "" || item.last.Before(conversations[oldest].last) {
						oldest = id
					}
				}
				delete(conversations, oldest)
			}
			entry.state = aiflow.NewConversation()
		}
		entry.last = time.Now()
		conversations[page] = entry
		mu.Unlock()
		return aiflow.Options{
			Load:         func(ctx context.Context) (aiflow.Settings, error) { return aiflow.Settings{API: v}, ctx.Err() },
			Open:         aiflow.OpenAIForDevice,
			Event:        func(context.Context, aiflow.Event) {},
			Conversation: entry.state,
		}, nil
	}, nil
}
