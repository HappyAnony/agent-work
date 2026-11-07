// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"

	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

var editOperationID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type EditHistory struct {
	OperationID string `json:"operationId"`
	Redo        bool   `json:"redo,omitempty"`
}

type EditRequest struct {
	ProtocolVersion    int                   `json:"protocolVersion"`
	StoreID            string                `json:"storeId"`
	PageID             string                `json:"pageId"`
	OperationID        string                `json:"operationId"`
	ExpectedSources    map[string]string     `json:"expectedSources,omitempty"`
	ExpectedTreeDigest string                `json:"expectedTreeDigest,omitempty"`
	Operations         []*editcore.Operation `json:"operations,omitempty"`
	History            *EditHistory          `json:"history,omitempty"`
	DryRun             bool                  `json:"dryRun,omitempty"`
	clipboardAssets    map[string][]byte
	tagRename          *uiTagRename
}

type EditResult struct {
	StoreID         string                       `json:"storeId"`
	PageID          string                       `json:"pageId"`
	Receipt         *sourcetx.Receipt            `json:"receipt,omitempty"`
	IndexFresh      bool                         `json:"indexFresh"`
	HistoryID       string                       `json:"historyId,omitempty"`
	Hashes          map[string]string            `json:"hashes,omitempty"`
	RequiredSources map[string]string            `json:"requiredSources,omitempty"`
	Operations      []*editcore.Operation        `json:"operations,omitempty"`
	Notices         []workspace.NativeEditNotice `json:"notices,omitempty"`
	Replay          bool                         `json:"replay,omitempty"`
}

func DecodeEditRequest(reader io.Reader) (EditRequest, error) {
	var r EditRequest
	if e := Decode(reader, &r); e != nil {
		return r, e
	}
	return r, r.Validate()
}

func (r EditRequest) Validate() error {
	if r.ProtocolVersion != 1 || r.StoreID == "" || len(r.StoreID) > 128 || !pageIdentity.MatchString(r.PageID) || !editOperationID.MatchString(r.OperationID) || len(r.ExpectedSources) > 1000 || (!r.DryRun && len(r.ExpectedSources) == 0 && r.tagRename == nil) || (r.ExpectedTreeDigest != "" && !hashPattern.MatchString(r.ExpectedTreeDigest)) {
		return ErrArgument
	}
	for name, h := range r.ExpectedSources {
		if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\\x00") || len(name) > 4096 || !hashPattern.MatchString(h) {
			return ErrArgument
		}
	}
	if r.tagRename != nil {
		return nil
	}
	if r.History != nil {
		if len(r.Operations) != 0 || !editOperationID.MatchString(r.History.OperationID) || r.History.OperationID == r.OperationID {
			return ErrArgument
		}
		return nil
	}
	if e := editcore.ValidateOperations(context.Background(), r.Operations); e != nil {
		return errors.Join(ErrArgument, e)
	}
	for _, op := range r.Operations {
		if op.RetData != nil {
			return ErrArgument
		}
	}
	return nil
}

// Edit is the sole original-operation orchestration for CLI and Server. It
// resolves and prepares the complete candidate under a read admission; the
// durable writer revalidates the captured read set under its publication lock.
func Edit(ctx context.Context, m *workspace.WorkspaceManager, r EditRequest) (EditResult, error) {
	return editOriginal(ctx, m, r, "", nil)
}

func editOriginal(ctx context.Context, m *workspace.WorkspaceManager, r EditRequest, historyDocument string, afterSource func() error, canonicalClientHash ...string) (out EditResult, err error) {
	if err = r.Validate(); err != nil {
		return out, err
	}
	canonical := r
	canonical.DryRun = false
	raw, err := json.Marshal(canonical)
	if err != nil {
		return out, ErrArgument
	}
	clientHash := sourcetx.Hash(raw)
	if len(canonicalClientHash) > 0 {
		clientHash = canonicalClientHash[0]
	}
	prefix := "pages/" + r.PageID + "/data/"
	scopeName := "wiki-edit:" + r.PageID
	if historyDocument != "" {
		scopeName = "wiki-editor-edit:" + prefix + historyDocument
	}
	scope := sourcetx.Hash([]byte(scopeName))
	finishIndex := func() error {
		if !out.Receipt.IndexFresh {
			if _, e := m.Rebuild(ctx, r.StoreID, "all"); e != nil {
				return errors.Join(sourcetx.ErrIndex, e)
			}
		}
		e := m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
			l, e := m.Acquire(c, r.StoreID, r.PageID)
			if e != nil {
				return e
			}
			defer l.Release()
			_, e = l.PrepareNativeReads(c, false)
			return e
		})
		if e != nil {
			return errors.Join(sourcetx.ErrIndex, e)
		}
		out.IndexFresh = true
		return nil
	}
	replay := func() (bool, error) {
		previous, found, e := m.LookupReceipt(ctx, r.StoreID, r.OperationID, clientHash)
		if e != nil || !found {
			return found, e
		}
		out = EditResult{StoreID: r.StoreID, PageID: r.PageID, Receipt: &previous, Replay: true}
		if !previous.SourceCommitted {
			return true, sourcetx.ErrConflict
		}
		hashes, e := m.CommittedHashes(ctx, r.StoreID, r.OperationID, r.PageID)
		if e != nil {
			return true, e
		}
		out.Hashes = map[string]string{}
		for name, h := range hashes {
			out.Hashes[strings.TrimPrefix(name, prefix)] = h
		}
		for name, h := range r.ExpectedSources {
			if _, changed := out.Hashes[name]; !changed {
				out.Hashes[name] = h
			}
		}
		if previous.HistoryScope == scope {
			out.HistoryID = previous.OperationID
		}
		if r.DryRun {
			previous.Status = "dry_run_replay"
			return true, nil
		}
		return true, finishIndex()
	}
	if done, e := replay(); done || e != nil {
		return out, e
	}
	tx := sourcetx.Request{OperationID: r.OperationID, ClientHash: clientHash}
	if r.History == nil {
		tx.HistoryScope = scope
	}
	var plan workspace.NativeEditPlan
	err = m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		tree, e := m.Tree(c, r.StoreID)
		if e != nil {
			return e
		}
		page, ok := tree.Get(r.PageID)
		if !ok || page.DeletedAt != nil {
			return os.ErrNotExist
		}
		tx.ExpectedTreeDigest = tree.SourceDigest()
		if r.ExpectedTreeDigest != "" && r.ExpectedTreeDigest != tx.ExpectedTreeDigest {
			return sourcetx.ErrConflict
		}
		for name, h := range r.ExpectedSources {
			raw, e := m.SourceFile(c, r.StoreID, prefix+name)
			if e != nil {
				return e
			}
			if sourcetx.Hash(raw) != h {
				return sourcetx.ErrConflict
			}
		}
		if r.History != nil {
			tx.Changes, e = m.HistoryChanges(c, r.StoreID, r.History.OperationID, r.PageID, r.History.Redo, scope)
			if errors.Is(e, sourcetx.ErrIdentity) {
				return ErrUnauthorized
			}
			if e != nil {
				return e
			}
			guards := &sourcetx.ReadGuards{Files: map[string]string{}}
			for name, h := range r.ExpectedSources {
				guards.Files[prefix+name] = h
			}
			tx.ReadGuards = guards
			if !r.History.Redo {
				if e = guardClipboardAssetUndo(c, m, r, &tx, prefix); e != nil {
					return e
				}
			}
		} else {
			l, e := m.Acquire(c, r.StoreID, r.PageID)
			if e != nil {
				return e
			}
			defer l.Release()
			if r.tagRename == nil {
				plan, e = l.PlanNativeEdits(c, r.Operations)
			} else {
				cfg, configErr := l.Configuration(c)
				if configErr != nil {
					return configErr
				}
				if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
					return ErrUnauthorized
				}
				plan, e = l.PlanNativeTagRename(c, r.tagRename.OldLabel, r.tagRename.NewLabel)
			}
			if e != nil {
				return e
			}
			if !r.DryRun || len(r.ExpectedSources) > 0 {
				for name, h := range plan.DirectSources {
					if r.ExpectedSources[name] != h {
						return sourcetx.ErrConflict
					}
				}
			}
			tx.Changes, tx.ReadGuards = plan.Changes, plan.ReadGuards
			if historyDocument != "" {
				if h := r.ExpectedSources[historyDocument]; h != "" {
					tx.ReadGuards.Files[prefix+historyDocument] = h
				}
			}
			if len(r.clipboardAssets) > 0 {
				cfg, e := l.Configuration(c)
				if e != nil {
					return e
				}
				if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
					return ErrUnauthorized
				}
				if e = stageClipboardAssets(c, &tx, prefix, historyDocument, r.clipboardAssets); e != nil {
					return e
				}
			}
		}
		tx.Noop = len(tx.Changes) == 0
		return c.Err()
	})
	if err != nil {
		if done, e := replay(); done {
			return out, e
		}
		return EditResult{}, err
	}
	var receipt sourcetx.Receipt
	if r.DryRun {
		receipt, err = m.Preview(ctx, r.StoreID, tx)
	} else {
		receipt, err = m.Apply(ctx, r.StoreID, tx)
	}
	if err != nil && !receipt.SourceCommitted {
		if done, e := replay(); done {
			return out, e
		}
		return EditResult{}, err
	}
	out = EditResult{StoreID: r.StoreID, PageID: r.PageID, Receipt: &receipt, RequiredSources: plan.DirectSources, Hashes: map[string]string{}, Operations: plan.Operations}
	for _, change := range tx.Changes {
		hash := ""
		if !change.Delete {
			hash = sourcetx.Hash(change.Data)
		}
		out.Hashes[strings.TrimPrefix(change.Path, prefix)] = hash
	}
	for name, h := range r.ExpectedSources {
		if _, changed := out.Hashes[name]; !changed {
			out.Hashes[name] = h
		}
	}
	if receipt.SourceCommitted && receipt.HistoryScope == scope {
		out.HistoryID = receipt.OperationID
	}
	if err != nil || r.DryRun {
		return out, err
	}
	if afterSource != nil {
		if e := afterSource(); e != nil {
			return out, errors.Join(sourcetx.ErrIndex, e)
		}
	}
	if e := finishIndex(); e != nil {
		return out, e
	}
	out.Notices = plan.Notices
	return out, nil
}
