package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func read(args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		if jsonOutput {
			if emit(stdout, map[string]any{"ok": false, "protocolVersion": 1, "error": wikiops.ErrorInfo(err)}) != 0 {
				return 1
			}
		} else {
			fmt.Fprintln(stderr, wikiops.ErrorInfo(err).Type+"; run wiki-cli --help.")
		}
		return 2
	}
	values := map[string]string{}
	for i := 0; i < len(args); i++ {
		name, value, inline := strings.Cut(args[i], "=")
		if name != "--root" && name != "--store" && name != "--page" && name != "--box" && name != "--doc" && name != "--block" {
			return fail(wikiops.ErrArgument)
		}
		if _, ok := values[name]; ok {
			return fail(wikiops.ErrArgument)
		}
		if !inline {
			i++
			if i == len(args) {
				return fail(wikiops.ErrArgument)
			}
			value = args[i]
		}
		if value == "" || strings.HasPrefix(value, "--") || value == "-h" {
			return fail(wikiops.ErrArgument)
		}
		values[name] = value
	}
	for _, name := range []string{"--root", "--store", "--page", "--box", "--doc"} {
		if values[name] == "" {
			return fail(wikiops.ErrArgument)
		}
	}
	manager := workspace.NewManager(workspace.Options{})
	ctx := context.Background()
	defer manager.Drain(ctx)
	id, err := manager.OpenStore(ctx, values["--root"])
	if err != nil {
		return fail(err)
	}
	if id != values["--store"] {
		return fail(workspace.ErrIdentity)
	}
	result, err := wikiops.Read(ctx, manager, wikiops.ReadRequest{StoreID: id, PageID: values["--page"], BoxID: values["--box"], DocID: values["--doc"], BlockID: values["--block"]})
	closeErr := manager.Drain(ctx)
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fail(err)
	}
	if jsonOutput {
		return emit(stdout, map[string]any{"ok": true, "protocolVersion": 1, "result": result})
	}
	if _, err = fmt.Fprintln(stdout, string(result.AST)); err != nil {
		return 1
	}
	return 0
}
