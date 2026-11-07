package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path"
	"strings"
	"time"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func bindTransactionHistoryPort(env *transactionEnvironment, c *legacyCommitContext) {
	h := &historyIndexContext{owner: c.r.owner, root: util.HistoryDir, workspace: c.r.workspace, queue: sql.CaptureLegacyHistoryQueue(), untitled: c.r.language(16)}
	env.moveHistory = func(ctx context.Context, tree *parse.Tree) error {
		return writeTransactionHistory(ctx, c, h, tree, time.Now(), func(name string) error { return h.index(ctx, name, c.parser()) })
	}
	env.outlineHistory = func(ctx context.Context, tree *parse.Tree) error {
		return writeTransactionHistory(ctx, c, h, tree, time.Now(), func(name string) error { return h.index(ctx, name, c.parser()) }, HistoryOpOutline)
	}
}

// Preserve the original timestamp/update directory and source bytes. File
// writes are sequential; a later failure retains the successful prefix.
func writeTransactionHistory(ctx context.Context, c *legacyCommitContext, h *historyIndexContext, tree *parse.Tree, now time.Time, index func(string) error, operations ...string) (err error) {
	guard := func() error {
		if err := c.r.guard(ctx); err != nil {
			return err
		}
		return h.guard(ctx)
	}
	if err = guard(); err != nil {
		return err
	}
	if tree == nil || tree.Root == nil || !ast.IsNodeIDPattern(tree.Box) {
		return errors.New("transaction_history_tree_invalid")
	}
	rel := strings.TrimPrefix(tree.Path, "/")
	if rel == "" || path.IsAbs(rel) || path.Clean(rel) != rel || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "\\") {
		return errors.New("transaction_history_path_invalid")
	}
	operation := HistoryOpUpdate
	if len(operations) > 0 {
		operation = operations[0]
		if operation != HistoryOpFormat && operation != HistoryOpOutline && operation != HistoryOpUpdate && operation != HistoryOpReplace {
			return errors.New("transaction_history_operation_invalid")
		}
	}
	name := now.Format("2006-01-02-150405") + "-" + operation
	err = func() (err error) {
		source, closeSource, err := c.r.openRepairIOChecked(ctx, false)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, closeSource()) }()
		if h.root == "" {
			return errors.New("transaction_history_root_unbound")
		}
		if err = os.MkdirAll(h.root, 0755); err != nil {
			return err
		}
		dest, err := os.OpenRoot(h.root)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, dest.Close()) }()
		copyFile := func(from, to string) error {
			if err := guard(); err != nil {
				return err
			}
			raw, err := source.Source.Read(ctx, from)
			if err != nil {
				return err
			}
			if err = guard(); err != nil {
				return err
			}
			if err = dest.MkdirAll(path.Dir(to), 0755); err != nil {
				return err
			}
			temp := to + ".tmp-" + ast.NewNodeID()
			f, err := dest.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				return err
			}
			_, err = f.Write(raw)
			err = errors.Join(err, f.Close())
			if err == nil {
				err = guard()
			}
			if err == nil {
				err = dest.Rename(temp, to)
			}
			if err != nil {
				return errors.Join(err, dest.Remove(temp))
			}
			return guard()
		}
		if err = copyFile(path.Join(tree.Box, rel), path.Join(name, tree.Box, rel)); err != nil {
			return err
		}
		for _, node := range tree.Root.ChildrenByType(ast.NodeAttributeView) {
			if operation == HistoryOpReplace {
				break
			} // Original replace history contains document bytes only.
			if !ast.IsNodeIDPattern(node.AttributeViewID) {
				return errors.New("transaction_history_av_invalid")
			}
			file := "storage/av/" + node.AttributeViewID + ".json"
			if err = copyFile(file, path.Join(name, file)); err != nil {
				return err
			}
		}
		return nil
	}()
	if err != nil {
		return err
	}
	if err = guard(); err != nil {
		return err
	}
	return index(name)
}
