// Temporary compatibility executable for the unconverted GUI (S06–S08).
package main

import (
	"fmt"
	"os"
	"wiki-kernel/internal/assetextract"
	"wiki-kernel/internal/legacy"
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

	if len(os.Args) > 1 && os.Args[1] == "serve" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}
	if err := legacy.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
