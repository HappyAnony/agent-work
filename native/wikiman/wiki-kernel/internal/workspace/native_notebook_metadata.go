// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"github.com/88250/lute/ast"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/notebooks"
)

func (l *Lease) NativeNotebookInfo(ctx context.Context, id string) (out *notebooks.Info, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		source := indexrepair.Source{Data: w.data}
		value, _, e := notebooks.Configuration(ctx, source, id, "auto")
		if e != nil {
			return e
		}
		formatter, e := r.configurationBusiness.HumanizeTime(r.configuration.Lang)
		if e != nil {
			return e
		}
		now := time.Now()
		out, e = notebooks.Information(ctx, source, id, value.Name, time.Local, func(t time.Time) string { return formatter(t, now) })
		return e
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (l *Lease) PlanNativeNotebookCreate(ctx context.Context, name, untitled string) (out notebooks.MetadataPlan, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configuration.FileTree == nil {
			return ErrDocument
		}
		var e error
		if w.data == nil {
			out, e = notebooks.PrepareEmptyCreate(ctx, "auto", name, untitled, ast.NewNodeID())
			return e
		}
		out, e = notebooks.PrepareCreate(ctx, indexrepair.Source{Data: w.data}, "auto", name, untitled, r.configuration.FileTree.Sort, ast.NewNodeID)
		return e
	})
	if err != nil {
		return notebooks.MetadataPlan{}, err
	}
	return out, nil
}

func (l *Lease) NativeNotebookConfiguration(ctx context.Context, id string) (out *conf.BoxConf, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		source := indexrepair.Source{Data: l.workspace.data}
		if e := source.Check(id, false); e != nil {
			return e
		}
		var e error
		out, _, e = notebooks.Configuration(ctx, source, id, "auto")
		return e
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
func (l *Lease) PlanNativeNotebookMetadata(ctx context.Context, q notebooks.MetadataRequest) (out notebooks.MetadataPlan, err error) {
	q.IDs = append([]string(nil), q.IDs...)
	q.Patch = append(json.RawMessage(nil), q.Patch...)
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		var e error
		out, e = notebooks.PrepareMetadata(ctx, indexrepair.Source{Data: l.workspace.data}, "auto", q)
		return e
	})
	if err != nil {
		return notebooks.MetadataPlan{}, err
	}
	return out, nil
}
