// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"context"
	"encoding/json"
	"github.com/88250/lute/ast"
	"strings"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/indexrepair"
)

func ReadAV(ctx context.Context, source indexrepair.Source, id string) (*av.AttributeView, error) {
	if !ast.IsNodeIDPattern(id) {
		return nil, indexrepair.ErrSourcePath
	}
	raw, e := source.Read(ctx, "storage/av/"+id+".json")
	if e != nil {
		return nil, e
	}
	var a *av.AttributeView
	if e = json.Unmarshal(raw, &a); e != nil {
		return nil, e
	}
	if a == nil || a.ID != id {
		return nil, indexrepair.ErrSourcePath
	}
	return a, ctx.Err()
}

func ReadAVData(ctx context.Context, source indexrepair.Source) ([][]byte, error) {
	entries, e := source.ReadDir(ctx, "storage/av")
	if indexrepair.IsSourceNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	out := [][]byte{}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !strings.HasSuffix(entry.Name(), ".json") || !ast.IsNodeIDPattern(id) {
			continue
		}
		raw, e := source.Read(ctx, "storage/av/"+entry.Name())
		if e != nil {
			return nil, e
		}
		var candidate *av.AttributeView
		if e = json.Unmarshal(raw, &candidate); e != nil {
			return nil, e
		}
		if candidate == nil || candidate.ID != id {
			return nil, indexrepair.ErrSourcePath
		}
		out = append(out, raw)
	}
	return out, ctx.Err()
}
