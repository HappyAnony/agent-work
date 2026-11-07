package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/internal/sourcetx"
)

func TestPortableServerOwnsSessionAndDrains(t *testing.T) {
	root := filepath.Join(t.TempDir(), "wiki ?# ' 中文")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(root, os.DirFS("../../../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	id := "f36f1884-2299-4fde-88c1-9da8f26d2aac"
	token := strings.Repeat("server-test-token-", 3)
	secret := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(secret, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- Run(ctx, []string{"--root=" + root, "--store", id, "--token-file", secret}, writer, &stderr)
		writer.Close()
	}()
	line, err := bufio.NewReader(reader).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var ready struct {
		Address   string
		OK        bool
		SessionID string
	}
	if json.Unmarshal(line, &ready) != nil || !ready.OK {
		t.Fatal(string(line))
	}
	req, _ := http.NewRequest("POST", "http://"+ready.Address+"/v1/content", strings.NewReader(`{"protocolVersion":1,"action":"document.get","storeId":"`+id+`","pageId":"1760000000000000002","boxId":"20261019090000-box0001","docId":"20261019090001-doc0001"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Wiki-Session", ready.SessionID)
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	source, err := sourcetx.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if w, err := source.Writer(context.Background(), false); err == nil {
		w.Close()
		t.Fatal("server did not hold writer session")
	}
	cancel()
	select {
	case status := <-done:
		if status != 0 || stderr.Len() != 0 {
			t.Fatal(status, stderr.String())
		}
	case <-time.After(8 * time.Second):
		t.Fatal("server shutdown leaked")
	}
	w, err := source.Writer(context.Background(), false)
	if err != nil {
		t.Fatal("server failed to release session", err)
	}
	w.Close()
}
func TestParseRejectsPublicBindingMissingAndDuplicateArguments(t *testing.T) {
	for _, args := range [][]string{{"--root", "--help"}, {"--root", "-h"}, {"--root=a", "--store=b", "--token-file=c", "--listen=0.0.0.0:1234"}, {"--root=a", "--root=b", "--store=b", "--token-file=c"}, {"--root=a", "--store=b", "--token-file=c", "--listen=localhost:12"}} {
		if _, err := Parse(args); err == nil {
			t.Fatal(args)
		}
	}
	secret := filepath.Join(t.TempDir(), "bad-secret")
	if err := os.WriteFile(secret, []byte(strings.Repeat("t", 32)), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := tokenFile(secret); err == nil {
		t.Fatal("world-readable token accepted")
	}
}
