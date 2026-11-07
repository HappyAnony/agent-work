// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/pageprefs"
)

type RecentDoc = pageprefs.RecentDoc
type OutlineDoc = pageprefs.OutlineDoc
type Criterion = pageprefs.Criterion
type CriterionTypes = pageprefs.CriterionTypes
type CriterionReplaceTypes = pageprefs.CriterionReplaceTypes

func storageRequest(action, key, sortBy string, keys []string, value interface{}) pageprefs.Request {
	raw, _ := json.Marshal(value)
	return pageprefs.Request{Action: action, Key: key, SortBy: sortBy, Keys: keys, Value: raw}
}
func RemoveRecentDoc(ids []string) {
	_, e := StorageContext(context.Background(), storageRequest("recent.remove", "", "", ids, nil))
	logOwnedTaskError("recent remove", e)
}
func setRecentDocByTree(t *parse.Tree) {
	d := RecentDoc{RootID: t.Root.ID, Icon: t.Root.IALAttr("icon"), Title: t.Root.IALAttr("title")}
	_, e := StorageContext(context.Background(), storageRequest("recent.record", "", "", nil, d))
	logOwnedTaskError("recent record", e)
}
func GetRecentDocs(sortBy string) ([]*RecentDoc, error) {
	v, e := StorageContext(context.Background(), storageRequest("recent.get", "", sortBy, nil, nil))
	if e != nil {
		return nil, e
	}
	return v.([]*RecentDoc), nil
}
func UpdateRecentDocOpenTime(id string) error {
	_, e := StorageContext(context.Background(), storageRequest("recent.open", id, "", nil, nil))
	return e
}
func UpdateRecentDocViewTime(id string) error {
	_, e := StorageContext(context.Background(), storageRequest("recent.view", id, "", nil, nil))
	return e
}
func UpdateRecentDocCloseTime(id string) error {
	_, e := StorageContext(context.Background(), storageRequest("recent.close", id, "", nil, nil))
	return e
}
func RemoveCriterion(name string) error {
	_, e := StorageContext(context.Background(), storageRequest("criteria.remove", name, "", nil, nil))
	return e
}
func SetCriterion(c *Criterion) error {
	_, e := StorageContext(context.Background(), storageRequest("criteria.set", "", "", nil, c))
	return e
}
func RemoveLocalStorageVals(keys []string) error {
	_, e := StorageContext(context.Background(), storageRequest("local.remove", "", "", keys, nil))
	return e
}
func SetLocalStorageVal(key string, val interface{}) error {
	_, e := StorageContext(context.Background(), storageRequest("local.setValue", key, "", nil, val))
	return e
}
func SetLocalStorage(val interface{}) error {
	_, e := StorageContext(context.Background(), storageRequest("local.set", "", "", nil, val))
	return e
}
func SetOutlineStorage(id string, val interface{}) error {
	_, e := StorageContext(context.Background(), storageRequest("outline.set", id, "", nil, val))
	return e
}
func RemoveOutlineStorage(id string) error {
	_, e := StorageContext(context.Background(), storageRequest("outline.remove", id, "", nil, nil))
	return e
}
func GetCriteria() []*Criterion {
	v, e := StorageContext(context.Background(), storageRequest("criteria.get", "", "", nil, nil))
	if e != nil {
		logOwnedTaskError("criteria", e)
		return []*Criterion{}
	}
	return v.([]*Criterion)
}
func GetLocalStorage() map[string]interface{} {
	v, e := StorageContext(context.Background(), storageRequest("local.get", "", "", nil, nil))
	if e != nil {
		logOwnedTaskError("local storage", e)
		return map[string]interface{}{}
	}
	return v.(map[string]interface{})
}
func GetOutlineStorage(id string) (map[string]interface{}, error) {
	v, e := StorageContext(context.Background(), storageRequest("outline.get", id, "", nil, nil))
	if e != nil {
		return nil, e
	}
	return v.(map[string]interface{}), nil
}
