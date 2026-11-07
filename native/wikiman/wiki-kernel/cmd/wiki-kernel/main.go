package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"wiki-kernel/internal/assetextract"
	"wiki-kernel/internal/buildinfo"
	"wiki-kernel/internal/handlers/cli"
	"wiki-kernel/internal/handlers/server"
	"wiki-kernel/internal/pdfdevice"
)

func main() {
	args := os.Args[1:]
	cliEntry := len(args) > 0 && args[0] == "--wiki-cli-entrypoint"
	if cliEntry {
		args = args[1:]
	}
	if len(args) == 1 && args[0] == assetextract.WorkerArgument {
		if err := assetextract.Serve(os.Stdin, os.Stdout); err != nil {
			os.Stderr.WriteString("asset_extraction_failed\n")
			os.Exit(2)
		}
		return
	}

	if len(args) == 1 && args[0] == pdfdevice.WorkerArgument {
		if err := pdfdevice.Serve(os.Stdin, os.Stdout); err != nil {
			os.Stderr.WriteString("pdf_device_failed\n")
			os.Exit(2)
		}
		return
	}

	if cliEntry {
		os.Exit(cli.RunWithInput(args, os.Stdin, os.Stdout, os.Stderr))
	}
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("wiki-kernel " + buildinfo.Version)
		return
	}
	if len(args) == 0 || len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("wiki-kernel serve --root PATH --store STORE_ID --token-file PRIVATE_FILE [--listen 127.0.0.1:0] [--app-dir TRUSTED_APP_RESOURCES] [--pandoc ABSOLUTE_EXECUTABLE_OR_ZIP] [--pdf-helper ABSOLUTE_EXECUTABLE]")
		fmt.Println("Optional --app-dir serves controlled portable editor assets read-only; must be separate from the knowledge source. No source directory listings, symlinks or legacy API fallback.")
		fmt.Println("Optional --ai-file and --preview-file enable device-scoped AI writing and previews. Use private files outside the knowledge source, bound to its store ID.")
		fmt.Println("Optional --cloud-account-file enables cloud account status, connection and refresh using a private device profile outside the knowledge source.")
		fmt.Println("DOCX export lazily uses --pandoc or the host architecture archive under app-dir/pandoc; PDF uses the macOS sibling wiki-pdf or --pdf-helper. Absent converters do not prevent editing. No downloads; native source unchanged.")
		fmt.Println("Portable local Server: shared page/content/query/edit/recover APIs, scoped assets and WebSocket events. Private token file must be mode 0600; no token in argv/URLs. JSON ready address on stdout, errors on stderr. Ctrl-C/SIGTERM drains contexts and releases the session. Pending journals require wiki-cli recover before startup.")
		fmt.Println("Transitional --workspace/--wd/--port/--fixedPort flags execute the sibling wiki-compat helper for the old GUI. Portable mode never imports legacy. Do not use the new source with the old GUI until S06–S08 integration.")
		return
	}
	compat, portable := false, false
	for _, arg := range args {
		if arg == "--workspace" || strings.HasPrefix(arg, "--workspace=") {
			compat = true
		}
		if arg == "--root" || strings.HasPrefix(arg, "--root=") {
			portable = true
		}
	}
	if compat && !portable {
		if err := execCompat(args); err != nil {
			fmt.Fprintln(os.Stderr, "compatibility helper unavailable")
			os.Exit(2)
		}
		return
	}
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	} else {
		fmt.Fprintln(os.Stderr, "Run wiki-kernel --help.")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(server.Run(ctx, args, os.Stdout, os.Stderr))
}
