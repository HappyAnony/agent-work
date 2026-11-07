package cli

import (
	"context"
	"io"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func query(args []string, input io.Reader, out io.Writer) int {
	fail := func(err error) int {
		if emit(out, map[string]any{"ok": false, "protocolVersion": 1, "error": wikiops.ErrorInfo(err)}) != 0 {
			return 1
		}
		return 2
	}
	values, err := operationArgs(args)
	if err != nil {
		return fail(err)
	}
	if values["--output"] != "" || values["--pandoc"] != "" || values["--pdf-helper"] != "" {
		return fail(wikiops.ErrArgument)
	}
	request, err := wikiops.DecodeQueryRequest(input)
	if err != nil {
		return fail(err)
	}
	ctx := context.Background()
	m := workspace.NewManager(workspace.Options{ReadDefaults: true})
	defer m.Drain(ctx)
	id, err := m.OpenStore(ctx, values["--root"])
	if err != nil {
		return fail(err)
	}
	if id != request.StoreID {
		return fail(workspace.ErrIdentity)
	}
	result, err := wikiops.Query(ctx, m, request)
	closeErr := m.Drain(ctx)
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fail(err)
	}
	return emit(out, map[string]any{"ok": true, "protocolVersion": 1, "result": result})
}
