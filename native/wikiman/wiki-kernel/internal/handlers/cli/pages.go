package cli

import (
	"context"
	"io"
	"strings"

	"wiki-kernel/internal/buildinfo"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func pages(args []string, stdin io.Reader, stdout io.Writer) int {
	fail := func(err error, result any) int {
		if emit(stdout, map[string]any{"ok": false, "protocolVersion": buildinfo.ProtocolVersion, "error": wikiops.ErrorInfo(err), "result": result}) != 0 {
			return 1
		}
		return 2
	}
	values := map[string]string{}
	for i := 0; i < len(args); i++ {
		name, value, inline := strings.Cut(args[i], "=")
		if (name != "--root" && name != "--request") || values[name] != "" {
			return fail(wikiops.ErrArgument, nil)
		}
		if !inline {
			i++
			if i == len(args) {
				return fail(wikiops.ErrArgument, nil)
			}
			value = args[i]
		}
		if value == "" || strings.HasPrefix(value, "--") || value == "-h" {
			return fail(wikiops.ErrArgument, nil)
		}
		values[name] = value
	}
	if values["--root"] == "" || values["--request"] != "-" {
		return fail(wikiops.ErrArgument, nil)
	}
	request, err := wikiops.DecodePageRequest(stdin)
	if err != nil {
		return fail(err, nil)
	}
	manager := workspace.NewManager(workspace.Options{ReadDefaults: request.Action == "check" && request.Scope == "content"})
	ctx := context.Background()
	defer manager.Drain(ctx)
	id, err := manager.OpenStore(ctx, values["--root"])
	if err != nil {
		return fail(err, nil)
	}
	if id != request.StoreID {
		return fail(workspace.ErrIdentity, nil)
	}
	result, err := wikiops.Pages(ctx, manager, request)
	closeErr := manager.Drain(ctx)
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fail(err, result)
	}
	return emit(stdout, map[string]any{"ok": true, "protocolVersion": buildinfo.ProtocolVersion, "result": result})
}
