// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
)

func requestBodyCancellation(ctx context.Context, body io.ReadCloser) func() error {
	done := make(chan struct{})
	var closing error
	stop := context.AfterFunc(ctx, func() { defer close(done); closing = body.Close() })
	return func() error {
		if !stop() {
			<-done
		}
		return closing
	}
}

type singleUpload struct {
	Name   string
	Data   []byte
	Values map[string][]string
}

func runSingleUpload[T any](ctx context.Context, request *http.Request, field string, limit int64, consume func(context.Context, *legacyReindexOwner, singleUpload) (T, error)) (out T, err error) {
	return runSingleUploadMode(ctx, request, field, limit, false, consume)
}

func runSingleUploadMode[T any](ctx context.Context, request *http.Request, field string, limit int64, optional bool, consume func(context.Context, *legacyReindexOwner, singleUpload) (T, error)) (out T, err error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return out, e
	}
	if request == nil || request.Body == nil || consume == nil || limit <= 0 {
		return out, errors.New("upload_request_unbound")
	}
	request.Body = http.MaxBytesReader(nil, request.Body, limit+(4<<20))
	var form *multipart.Form
	err = base.runFinalized(ctx, func(c, life context.Context) (result error) {
		finishBody := requestBodyCancellation(c, request.Body)
		defer func() { result = errors.Join(result, finishBody()) }()
		if e := request.ParseMultipartForm(32 << 20); e != nil {
			if optional && errors.Is(e, http.ErrNotMultipart) && c.Err() == nil {
				request.MultipartForm = &multipart.Form{Value: request.PostForm}
			} else {
				form = request.MultipartForm
				return errors.Join(e, c.Err())
			}
		}
		form = request.MultipartForm
		if form == nil || len(form.File[field]) > 1 || (!optional && len(form.File[field]) != 1) {
			return errors.New("invalid upload file")
		}
		selected := singleUpload{Values: map[string][]string{}}
		if len(form.File[field]) == 1 {
			header := form.File[field][0]
			if header.Size < 0 || header.Size > limit {
				return errors.New("upload_size_budget")
			}
			reader, e := header.Open()
			if e != nil {
				return e
			}
			data, e := io.ReadAll(io.LimitReader(extensionGuardedBody{ReadCloser: reader, guard: func() error { return base.guard(c) }}, limit+1))
			e = errors.Join(e, reader.Close())
			if e != nil {
				return e
			}
			if int64(len(data)) != header.Size || int64(len(data)) > limit {
				return errors.New("upload_size_changed")
			}
			selected.Name, selected.Data = header.Filename, data
		}
		var e error
		for key, values := range form.Value {
			selected.Values[key] = append([]string(nil), values...)
		}
		if e = form.RemoveAll(); e != nil {
			return e
		}
		form = nil
		if e = base.guard(c); e != nil {
			return e
		}
		out, e = consume(c, base, selected)
		return e
	}, func(error) error {
		if form != nil {
			return form.RemoveAll()
		}
		return nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return
}
