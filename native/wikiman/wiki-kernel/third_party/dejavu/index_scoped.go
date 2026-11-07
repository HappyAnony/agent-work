// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/dejavu/util"
	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"io/fs"
	"time"
)

func (repo *Repo) index0Scoped(memo string, checkChunks bool, context map[string]interface{}) (ret *entity.Index, err error) {
	var files []*entity.File
	ignoreMatcher := repo.ignoreMatcher()
	repo.emit(eventbus.EvtIndexBeforeWalkData, context, repo.DataPath)
	start := time.Now()
	err = filelock.Walk(repo.DataPath, func(path string, d fs.DirEntry, err error) error {
		if nil != err {
			if isNoSuchFileOrDirErr(err) {
				// An error `Failed to create data snapshot` is occasionally reported during automatic data sync https://github.com/siyuan-note/siyuan/issues/8998
				logging.LogInfof("ignore not exist err [%s]", err)
				return nil
			}
			logging.LogErrorf("walk data failed: %s", err)
			return err
		}

		if e := repo.store.check(); e != nil {
			return e
		}
		info, err := d.Info()
		if nil != err {
			logging.LogErrorf("walk data failed: %s", err)
			return err
		}
		if ignored, ignoreErr := repo.builtInIgnore(info, path); ignored || nil != ignoreErr {
			return ignoreErr
		}

		p := repo.relPath(path)
		if ignoreMatcher.MatchesPath(p) {
			return nil
		}

		files = append(files, entity.NewFile(p, info.Size(), info.ModTime().UnixMilli()))
		repo.emit(eventbus.EvtIndexWalkData, context, p)
		return nil
	})
	if nil != err {
		logging.LogErrorf("walk data failed: %s", err)
		return
	}
	logging.LogInfof("walk data [files=%d] cost [%s]", len(files), time.Since(start))
	//sort.Slice(files, func(i, j int) bool { return files[i].Updated > files[j].Updated })
	//for _, f := range files {
	//	logging.LogInfof("walked data [file=%s]", f.Path)
	//}
	if 1 > len(files) {
		// 如果没有文件，则不创建快照 Abandon snapshot if file does not exist when creating snapshot https://github.com/siyuan-note/siyuan/issues/9948
		err = ErrEmptyIndex
		logging.LogErrorf("empty index [%s]", repo.DataPath)
		return
	}

	latest, err := repo.Latest()
	init := false
	if nil != err {
		if ErrNotFoundIndex != err {
			logging.LogErrorf("get latest index failed: %s", err)
			return
		}

		// 如果没有索引，则创建第一个索引
		latest = &entity.Index{
			ID:         util.RandHash(),
			Memo:       memo,
			Created:    time.Now().UnixMilli(),
			SystemID:   repo.DeviceID,
			SystemName: repo.DeviceName,
			SystemOS:   repo.DeviceOS,
		}
		init = true
	}

	var latestFiles []*entity.File
	if !init {
		latestFiles, err = repo.scopedLatestFiles(latest, checkChunks, context)
		if err != nil {
			return
		}
	}
	// Hash every source file. Metadata-only comparisons miss same-size edits with
	// preserved timestamps, and old path+second identities alias distinct bytes.
	oldIndexing, oldPrevious := repo.snapshot.indexing, repo.snapshot.previous
	repo.snapshot.indexing = true
	repo.snapshot.previous = map[string]*entity.File{}
	for _, f := range latestFiles {
		repo.snapshot.previous[f.Path] = f
	}
	defer func() { repo.snapshot.indexing = oldIndexing; repo.snapshot.previous = oldPrevious }()
	repo.emit(eventbus.EvtIndexUpsertFiles, context, len(files))
	if err = repo.scopedIndexUpserts(files, context); err != nil {
		return
	}
	changed := init || len(files) != len(latestFiles)
	if !changed {
		for _, f := range files {
			old := repo.snapshot.previous[f.Path]
			if old == nil || old.ID != f.ID {
				changed = true
				break
			}
		}
	}
	if !changed {
		ret = latest
		return
	}

	if init {
		ret = latest
	} else {
		ret = &entity.Index{
			ID:         util.RandHash(),
			Memo:       memo,
			Created:    time.Now().UnixMilli(),
			SystemID:   repo.DeviceID,
			SystemName: repo.DeviceName,
			SystemOS:   repo.DeviceOS,
		}
	}

	for _, file := range files {
		ret.Files = append(ret.Files, file.ID)
		ret.Size += file.Size
	}
	ret.Count = len(ret.Files)

	err = repo.store.PutIndex(ret)
	if nil != err {
		logging.LogErrorf("put index failed: %s", err)
		return
	}

	if err = repo.validateSnapshotClosure(ret); err != nil {
		return
	}
	err = repo.updateLatestScoped(ret)
	if nil != err {
		logging.LogErrorf("update latest failed: %s", err)
		return
	}
	return
}
