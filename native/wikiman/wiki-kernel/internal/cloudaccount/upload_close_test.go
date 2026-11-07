// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudaccount

import (
	"context"
	"errors"
	"io"
	"testing"
)

type uploadCloseFault struct {
	reads, closes     int
	readErr, closeErr error
	cancel            context.CancelFunc
}

func (f *uploadCloseFault) Read(b []byte) (int, error) {
	f.reads++
	if f.readErr != nil {
		return 0, f.readErr
	}
	return copy(b, []byte("data")), io.EOF
}
func (f *uploadCloseFault) Close() error {
	f.closes++
	if f.cancel != nil {
		f.cancel()
	}
	return f.closeErr
}

func TestCloudUploadInvalidSizeAndReadCloseErrorsPreserved(t *testing.T) {
	for _, kind := range []string{"negative", "oversize", "read-close"} {
		t.Run(kind, func(t *testing.T) {
			state, o := fixture(t)
			state.User = cached(o)
			o.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
				return tokenReply("synthetic"), nil
			}
			uploads := 0
			o.Upload = func(context.Context, int, string, UploadFile, string, string) (Response, error) {
				uploads++
				return successfulBusiness(), nil
			}
			s := openService(t, *o)
			primary, closing := errors.New("read failed"), errors.New("close failed")
			file := &uploadCloseFault{closeErr: closing}
			size := int64(4)
			switch kind {
			case "negative":
				size = -1
			case "oversize":
				size = 11 << 20
			case "read-close":
				file.readErr = primary
			}
			ports := filePorts()
			ports.Open = func(context.Context, string) (io.ReadCloser, int64, error) { return file, size, nil }
			end := 0
			ports.Event = func(ctx context.Context, event UploadEvent) error {
				if event.Kind == "end" {
					end++
					if ctx.Err() != nil {
						t.Error("cleanup context cancelled")
					}
				}
				return nil
			}
			n, err := s.UploadAssets(context.Background(), []string{"asset"}, ExportForumBusiness, ports)
			if n != 0 || uploads != 0 || !errors.Is(err, closing) || file.closes != 1 || end != 1 {
				t.Fatal(n, uploads, err, file.closes, end)
			}
			if kind == "negative" && !errors.Is(err, ErrAssetChanged) {
				t.Fatal(err)
			}
			if kind == "read-close" && !errors.Is(err, primary) {
				t.Fatal(err)
			}
			if kind != "read-close" && file.reads != 0 {
				t.Fatal("skip path read payload", file.reads)
			}
		})
	}
}

func TestCloudUploadCloseCancellationAndEndErrorsJoin(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	o.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
		return tokenReply("synthetic"), nil
	}
	uploads := 0
	o.Upload = func(context.Context, int, string, UploadFile, string, string) (Response, error) {
		uploads++
		return successfulBusiness(), nil
	}
	s := openService(t, *o)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closing, ending := errors.New("close failed"), errors.New("end failed")
	file := &uploadCloseFault{closeErr: closing, cancel: cancel}
	ports := filePorts()
	ports.Open = func(context.Context, string) (io.ReadCloser, int64, error) { return file, 4, nil }
	end := 0
	ports.Event = func(ctx context.Context, event UploadEvent) error {
		if event.Kind == "end" {
			end++
			if ctx.Err() != nil {
				t.Error("cleanup context cancelled")
			}
			return ending
		}
		return nil
	}
	n, err := s.UploadAssets(ctx, []string{"asset"}, ExportForumBusiness, ports)
	if n != 0 || uploads != 0 || file.closes != 1 || end != 1 || !errors.Is(err, closing) || !errors.Is(err, ending) || !errors.Is(err, context.Canceled) {
		t.Fatal(n, uploads, err, file.closes, end)
	}
}
