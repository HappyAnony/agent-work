package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func operationArgs(args []string) (map[string]string, error) {
	values := map[string]string{}
	for i := 0; i < len(args); i++ {
		name, value, inline := strings.Cut(args[i], "=")
		if (name != "--root" && name != "--request" && name != "--output" && name != "--pandoc" && name != "--pdf-helper") || values[name] != "" {
			return nil, wikiops.ErrArgument
		}
		if !inline {
			i++
			if i == len(args) {
				return nil, wikiops.ErrArgument
			}
			value = args[i]
		}
		if value == "" || strings.HasPrefix(value, "-") && !(name == "--request" && value == "-") {
			return nil, wikiops.ErrArgument
		}
		values[name] = value
	}
	if values["--root"] == "" || values["--request"] != "-" || (values["--output"] != "" && values["--output"] != "text" && values["--output"] != "ast") {
		return nil, wikiops.ErrArgument
	}
	return values, nil
}
func operation(command string, args []string, input io.Reader, out io.Writer) int {
	fail := func(err error, result any) int {
		if emit(out, map[string]any{"ok": false, "protocolVersion": 1, "error": wikiops.ErrorInfo(err), "result": result}) != 0 {
			return 1
		}
		return 2
	}
	values, err := operationArgs(args)
	if err != nil {
		return fail(err, nil)
	}
	ctx := context.Background()
	if command == "status" || command == "recover" {
		if values["--output"] != "" || values["--pandoc"] != "" || values["--pdf-helper"] != "" {
			return fail(wikiops.ErrArgument, nil)
		}
		if command == "status" {
			var request wikiops.StatusRequest
			if err := wikiops.Decode(input, &request); err != nil {
				return fail(err, nil)
			}
			result, err := wikiops.StatusRoot(ctx, values["--root"], request)
			if err != nil {
				return fail(err, result)
			}
			return emit(out, map[string]any{"ok": true, "protocolVersion": 1, "result": result})
		}
		var request wikiops.RecoverRequest
		if err := wikiops.Decode(input, &request); err != nil {
			return fail(err, nil)
		}
		if err := request.Validate(); err != nil {
			return fail(err, nil)
		}
		result, err := wikiops.RecoverRoot(ctx, values["--root"], request)
		if err != nil {
			return fail(err, result)
		}
		return emit(out, map[string]any{"ok": true, "protocolVersion": 1, "result": result})
	}
	request, err := wikiops.DecodeContentRequest(input)
	if err != nil {
		return fail(err, nil)
	}
	if values["--pandoc"] != "" {
		if request.Action != "document.export" || request.Format != "docx" {
			return fail(wikiops.ErrArgument, nil)
		}
		ctx = wikiops.WithPandoc(ctx, values["--pandoc"], values["--root"])
	}
	if values["--pdf-helper"] != "" && (request.Action != "document.export" || request.Format != "pdf") {
		return fail(wikiops.ErrArgument, nil)
	}
	if request.Action == "document.export" && request.Format == "pdf" {
		helper := values["--pdf-helper"]
		if helper == "" {
			if executable, e := os.Executable(); e == nil {
				helper = filepath.Join(filepath.Dir(executable), "wiki-pdf")
			}
		}
		ctx = wikiops.WithPDF(ctx, helper, values["--root"])
	}
	if values["--output"] != "" && request.Action != "document.get" && request.Action != "block.get" {
		return fail(wikiops.ErrArgument, nil)
	}
	if values["--output"] != "" {
		if request.Format != "" && request.Format != values["--output"] {
			return fail(wikiops.ErrArgument, nil)
		}
		request.Format = values["--output"]
	}
	m := workspace.NewManager(workspace.Options{ReadDefaults: true})
	defer m.Drain(ctx)
	id, err := m.OpenStore(ctx, values["--root"])
	if err != nil {
		return fail(err, nil)
	}
	if id != request.StoreID {
		return fail(workspace.ErrIdentity, nil)
	}
	result, err := wikiops.Content(ctx, m, request)
	closeErr := m.Drain(ctx)
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fail(err, result)
	}
	if values["--output"] != "" {
		value := result.Text
		if values["--output"] == "ast" {
			value = string(result.AST)
		}
		if _, err := fmt.Fprint(out, value); err != nil {
			return 1
		}
		return 0
	}
	return emit(out, map[string]any{"ok": true, "protocolVersion": 1, "result": result})
}
