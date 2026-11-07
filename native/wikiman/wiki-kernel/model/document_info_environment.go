package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"os"
	"path/filepath"
	"wiki-kernel/av"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

type documentInfoEnvironment struct {
	ctx      context.Context
	services docquery.InfoServices
}

func legacyDocumentInfoEnvironment(batch bool) *documentInfoEnvironment {
	return &documentInfoEnvironment{ctx: context.Background(), services: docquery.InfoServices{
		Wait: func(context.Context) error { FlushTxQueue(); return nil },
		Trees: func(_ context.Context, ids []string) (map[string]*parse.Tree, error) {
			if batch {
				return filesys.LoadTrees(ids), nil
			}
			ret := map[string]*parse.Tree{}
			for _, id := range ids {
				tree, e := LoadTreeByBlockID(id)
				if e != nil {
					logging.LogErrorf("load tree by root id [%s] failed: %s", id, e)
					continue
				}
				ret[id] = tree
			}
			return ret, nil
		},
		Exists: func(_ context.Context, id string) (bool, error) { return treenode.ExistBlockTree(id), nil },
		References: func(_ context.Context, id, root string) ([]string, error) {
			defs := queryBlockRefDefs(treenode.GetBlockTree(id))
			buildBacklinkListItemRefs(defs)
			var ids []string
			for _, d := range defs {
				ids = append(ids, d.RefID)
			}
			return ids, nil
		},
		AVName:        func(_ context.Context, id string) (string, error) { return av.GetAttributeViewName(id) },
		DefaultAVName: func(context.Context) (string, error) { return Conf.language(105), nil },
		Subfiles: func(_ context.Context, t *parse.Tree) (int, error) {
			entries, e := os.ReadDir(filepath.Join(util.DataDir, t.Box, util.StripDocExt(t.Path)))
			if e != nil {
				return 0, e
			}
			count := 0
			for _, f := range entries {
				if util.HasDocExt(f.Name()) {
					count++
				}
			}
			return count, nil
		},
		IgnoreAVErrors: true, IgnoreSingleDirectoryError: true, Warn: func(_, raw string) { logging.LogWarnf("invalid scroll data [%s]", raw) },
	}}
}
func (tx *Transaction) documentInfos(q docquery.InfoRequest) ([]*docquery.Info, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		e := tx.environment.documentInfo
		if e == nil {
			return nil, docquery.ErrInfoDependency
		}
		return docquery.Infos(tx.environment.ctx, q, e.services)
	}
	e := legacyDocumentInfoEnvironment(q.Batch)
	return docquery.Infos(e.ctx, q, e.services)
}
func legacyPromoteRefDefs(defs []*RefDefs) (map[string]string, error) {
	if Conf == nil || Conf.Search == nil {
		return nil, errors.New("document_info_configuration_unbound")
	}
	return docquery.PromoteRefDefs(context.Background(), defs, docquery.RefPromotionServices{CaseSensitive: Conf.Search.CaseSensitive,
		Blocks: func(_ context.Context, ids []string) ([]indexdb.BlockRecord, error) {
			rows := sql.GetBlocks(ids)
			var ret []indexdb.BlockRecord
			for _, row := range rows {
				if row != nil {
					ret = append(ret, indexdb.BlockRecord(*row))
				}
			}
			return ret, nil
		},
		Parser: func(context.Context) (*lute.Lute, error) { return util.NewLute(), nil },
	})
}
