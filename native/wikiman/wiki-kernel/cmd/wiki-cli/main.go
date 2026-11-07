package main

import (
	"os"

	"wiki-kernel/internal/assetextract"
	"wiki-kernel/internal/handlers/cli"
	"wiki-kernel/internal/pdfdevice"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == assetextract.WorkerArgument {
		if err := assetextract.Serve(os.Stdin, os.Stdout); err != nil {
			os.Stderr.WriteString("asset_extraction_failed\n")
			os.Exit(2)
		}
		return
	}

	if len(os.Args) == 2 && os.Args[1] == pdfdevice.WorkerArgument {
		if err := pdfdevice.Serve(os.Stdin, os.Stdout); err != nil {
			os.Stderr.WriteString("pdf_device_failed\n")
			os.Exit(2)
		}
		return
	}
	os.Exit(cli.RunWithInput(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
