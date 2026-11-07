// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/internal/avquery"
)

func originalAttributeKeys(avID, kind, keyword string, ids []string) (ret []*av.Key) {
	ret = []*av.Key{}
	view, e := av.ParseAttributeView(avID)
	if e != nil {
		logging.LogErrorf("parse attribute view [%s] failed: %s", avID, e)
		return
	}
	var keys []*av.Key
	for _, kv := range view.KeyValues {
		keys = append(keys, kv.Key)
	}
	ret, e = avquery.SelectKeys(context.Background(), keys, func(k *av.Key) avquery.KeyMeta {
		return avquery.KeyMeta{ID: k.ID, Name: k.Name, Type: string(k.Type), Relation: k.Relation != nil}
	}, kind, keyword, ids)
	if e != nil {
		logging.LogErrorf("attribute keys failed: %s", e)
	}
	return
}
