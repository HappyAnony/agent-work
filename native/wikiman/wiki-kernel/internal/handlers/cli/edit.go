package cli

import (
	"context"
	"io"

	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func edit(args []string, input io.Reader, out io.Writer) int {
	fail := func(err error, result any) int {
		if emit(out, map[string]any{"ok": false, "protocolVersion": 1, "result": result, "error": wikiops.ErrorInfo(err)}) != 0 {
			return 1
		}
		return 2
	}
	values, err := operationArgs(args)
	if err != nil {
		return fail(err, nil)
	}
	if values["--output"] != "" || values["--pandoc"] != "" || values["--pdf-helper"] != "" {
		return fail(wikiops.ErrArgument, nil)
	}
	request, err := wikiops.DecodeEditRequest(input)
	if err != nil {
		return fail(err, nil)
	}
	ctx := context.Background()
	m := workspace.NewManager(workspace.Options{ReadDefaults: true})
	defer m.Drain(ctx)
	id, err := m.OpenStore(ctx, values["--root"])
	if err != nil {
		return fail(err, nil)
	}
	if id != request.StoreID {
		return fail(workspace.ErrIdentity, nil)
	}
	result, err := wikiops.Edit(ctx, m, request)
	closeErr := m.Drain(ctx)
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fail(err, result)
	}
	return emit(out, map[string]any{"ok": true, "protocolVersion": 1, "result": result})
}
