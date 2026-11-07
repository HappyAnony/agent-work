//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/conf"
	"wiki-kernel/filesys"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func TestLegacyAnchorRefreshUsesCapturedOwnerAndCommitsSourceThenIndex(t *testing.T) {
	c, r, usingFile, _, _ := conflictFixture(t)
	ctx := context.Background()
	defID := "20261028140002-refdef1"
	defTreeInfo, err := r.referenceLookup(ctx, defID)
	if err != nil || defTreeInfo == nil {
		t.Fatal(defTreeInfo, err)
	}
	defTree, err := r.loadRepairTree(ctx, defTreeInfo.BoxID, defTreeInfo.Path, util.NewLute())
	if err != nil {
		t.Fatal(err)
	}
	def := treenode.GetNodeInTree(defTree, defID)
	if def == nil {
		t.Fatal("definition node missing")
	}
	def.SetIALAttr("name", "captured anchor")
	owner, err := captureLegacyAnchorContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	notices := make(chan commitNotice, 2)
	owner.commit.publish = func(notice commitNotice) { notices <- notice }
	if err = owner.refresh(ctx, map[string]*ast.Node{defID: def}, map[string]*parse.Tree{defTree.ID: defTree}); err != nil {
		t.Fatal(err)
	}
	select {
	case notice := <-notices:
		if notice.kind != "anchor" || notice.change.DefinitionID != defID {
			t.Fatal("unexpected owned anchor notice", notice)
		}
	case <-time.After(time.Second):
		t.Fatal("owned anchor notice missing")
	}

	usingID := strings.TrimSuffix(filepath.Base(usingFile), util.DocExt)
	usingTree, err := filesys.LoadTree(defTreeInfo.BoxID, "/"+usingID+util.DocExt, util.NewLute())
	if err != nil {
		t.Fatal(err)
	}
	var anchorText string
	ast.Walk(usingTree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if entering && n.Type == ast.NodeTextMark && n.IsTextMarkType("block-ref") {
			anchorText = n.TextMarkTextContent
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	if anchorText != "captured anchor" {
		t.Fatal("source anchor not updated", anchorText)
	}
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		var content string
		if err = r.main.DB.QueryRowContext(ctx, "SELECT content FROM "+table+" WHERE id = ?", "20261028140004-refpar1").Scan(&content); err != nil || !strings.Contains(content, "captured anchor") {
			t.Fatalf("%s did not receive committed anchor: %q, %v", table, content, err)
		}
	}
	var refCount int
	if err = r.main.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM refs WHERE def_block_id = ?", defID).Scan(&refCount); err != nil || refCount != 1 {
		t.Fatal("reference projection changed", refCount, err)
	}

	beforeStale, err := os.ReadFile(usingFile)
	if err != nil {
		t.Fatal(err)
	}
	updateRefTextRenameDocLock.Lock()
	updateRefTextRenameDocs = map[string]queuedLegacyAnchorRefresh{}
	updateRefTextRenameDocLock.Unlock()
	if err = updateRefTextRenameDocContext(ctx, defTree); err != nil {
		t.Fatal(err)
	}
	oldConf := Conf
	Conf = NewAppConf()
	err = flushUpdateRefTextRenameDocContext(ctx)
	Conf = oldConf
	if !errors.Is(err, conf.ErrBusinessUnbound) {
		t.Fatal("queued refresh followed replacement owner", err)
	}
	afterStale, err := os.ReadFile(usingFile)
	if err != nil || !bytes.Equal(beforeStale, afterStale) {
		t.Fatal("stale owner changed source", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = captureLegacyAnchorContext(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled owner captured", err)
	}
	def.SetIALAttr("name", "close-canceled anchor")
	if err = owner.refresh(ctx, map[string]*ast.Node{defID: def}, map[string]*parse.Tree{defTree.ID: defTree}); err != nil {
		t.Fatal(err)
	}
	if err = c.CloseCloudRepo(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case notice := <-notices:
		t.Fatal("closed owner published delayed notice", notice)
	case <-time.After(300 * time.Millisecond):
	}
	recordCloudBusinessEvidence(t, "legacy-anchor-owner.json", map[string]interface{}{"actualSourceUpdated": true, "threeIndexTablesCommitted": true, "referenceProjectionPreserved": true, "ownedNoticeAfterCommit": true, "closeCanceledDelayedNotice": true, "capturedOwnerRejectedReplacement": true, "staleSourceUnchanged": true, "canceledBeforeCapture": true})
}
