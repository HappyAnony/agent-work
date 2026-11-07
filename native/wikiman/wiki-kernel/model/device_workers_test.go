package model

import (
	"os"
	"testing"
	"wiki-kernel/internal/assetextract"
	"wiki-kernel/internal/pdfdevice"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == assetextract.WorkerArgument {
		if e := assetextract.Serve(os.Stdin, os.Stdout); e != nil {
			os.Stderr.WriteString("asset_extraction_failed\n")
			os.Exit(2)
		}
		os.Exit(0)
	}

	if len(os.Args) == 2 && os.Args[1] == pdfdevice.WorkerArgument {
		if e := pdfdevice.Serve(os.Stdin, os.Stdout); e != nil {
			os.Stderr.WriteString("pdf_device_failed\n")
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
