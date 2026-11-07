// Package server is the explicit portable listener lifecycle. Importing it does
// not initialize a source, listener, legacy worker or global configuration.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"wiki-kernel/internal/handlers/readhttp"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

type Config struct{ Root, StoreID, TokenFile, Listen, AppDir, Pandoc, PDFHelper, AIFile, PreviewFile string }

func Parse(args []string) (Config, error) {
	c := Config{Listen: "127.0.0.1:0"}
	values := map[string]string{}
	for i := 0; i < len(args); i++ {
		name, value, inline := strings.Cut(args[i], "=")
		switch name {
		case "--root", "--store", "--token-file", "--listen", "--app-dir", "--pandoc", "--pdf-helper", "--ai-file", "--preview-file":
		default:
			return c, wikiops.ErrArgument
		}
		if values[name] != "" {
			return c, wikiops.ErrArgument
		}
		if !inline {
			i++
			if i == len(args) {
				return c, wikiops.ErrArgument
			}
			value = args[i]
		}
		if value == "" || strings.HasPrefix(value, "-") {
			return c, wikiops.ErrArgument
		}
		values[name] = value
	}
	c.Root = values["--root"]
	c.StoreID = values["--store"]
	c.TokenFile = values["--token-file"]
	c.AppDir = values["--app-dir"]
	c.Pandoc = values["--pandoc"]
	c.PDFHelper = values["--pdf-helper"]
	c.AIFile = values["--ai-file"]
	c.PreviewFile = values["--preview-file"]
	if values["--listen"] != "" {
		c.Listen = values["--listen"]
	}
	host, port, err := net.SplitHostPort(c.Listen)
	ip := net.ParseIP(host)
	number, numberErr := strconv.Atoi(port)
	if c.Root == "" || c.StoreID == "" || c.TokenFile == "" || err != nil || ip == nil || !ip.IsLoopback() || numberErr != nil || number < 0 || number > 65535 {
		return c, wikiops.ErrArgument
	}
	return c, nil
}
func tokenFile(name string) (string, error) {
	return readPrivateFile(name, 4096)
}

func readPrivateFile(name string, limit int64) (string, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > limit {
		return "", wikiops.ErrArgument
	}
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", wikiops.ErrArgument
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return "", wikiops.ErrArgument
	}
	return strings.TrimSpace(string(raw)), nil
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		_ = json.NewEncoder(stderr).Encode(map[string]any{"ok": false, "error": wikiops.ErrorInfo(err)})
		return 2
	}
	config, err := Parse(args)
	if err != nil {
		return fail(err)
	}
	token, err := tokenFile(config.TokenFile)
	if err != nil {
		return fail(err)
	}
	var assets *editorAssets
	if config.AppDir != "" {
		assets, err = openEditorAssets(config.AppDir, config.Root)
		if err != nil {
			return fail(err)
		}
		defer assets.root.Close()
	}
	ai, err := deviceAI(config)
	if err != nil {
		return fail(err)
	}
	m := workspace.NewManager(workspace.Options{ReadDefaults: true, AIOptions: ai})
	defer m.Drain(context.Background())
	handler, err := readhttp.NewSession(m, token, assets.themes()...)
	if err != nil {
		return fail(err)
	}
	defer handler.Close()
	id, err := m.OpenStore(ctx, config.Root)
	if err != nil {
		return fail(err)
	}
	if id != config.StoreID {
		return fail(workspace.ErrIdentity)
	}
	if err := m.ClaimSession(ctx, id, false); err != nil {
		return fail(err)
	}
	sessionID := handler.BindStore(config.Root, id)
	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return fail(err)
	}
	defer listener.Close()
	pandoc := config.Pandoc
	if pandoc == "" && config.AppDir != "" {
		pandoc = filepath.Join(config.AppDir, "pandoc", "pandoc-"+runtime.GOOS+"-"+runtime.GOARCH+".zip")
		pandoc, _ = filepath.Abs(pandoc)
	}
	conversionContext := wikiops.WithPandoc(ctx, pandoc, config.Root)
	helper := config.PDFHelper
	if helper == "" {
		if executable, e := os.Executable(); e == nil {
			helper = filepath.Join(filepath.Dir(executable), "wiki-pdf")
		}
	}
	conversionContext = wikiops.WithPDF(conversionContext, helper, config.Root)
	conversionContext, err = devicePreview(conversionContext, config)
	if err != nil {
		_ = listener.Close()
		return fail(err)
	}
	service := &http.Server{BaseContext: func(net.Listener) context.Context { return conversionContext }, Handler: portableHandler(handler, assets, listener.Addr().String()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- service.Serve(listener) }()
	if err := json.NewEncoder(stdout).Encode(map[string]any{"ok": true, "type": "ready", "protocolVersion": 1, "storeId": id, "address": listener.Addr().String(), "sessionId": sessionID, "editorAssets": assets != nil}); err != nil {
		service.Close()
		<-done
		return 1
	}
	select {
	case <-ctx.Done():
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(stderr, "portable server stopped")
			handler.Close()
			return 2
		}
		return 0
	}
	handler.Close()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := service.Shutdown(shutdown); err != nil {
		_ = service.Close()
	}
	<-done
	if err := m.Drain(shutdown); err != nil {
		return fail(err)
	}
	return 0
}
