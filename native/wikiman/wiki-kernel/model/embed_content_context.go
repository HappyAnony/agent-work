// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/task"
)

const embedContentTimeout = 30 * time.Second

// IndexEmbedBlockJobContext captures the workspace before the task is
// admitted. The request may finish, but workspace shutdown still cancels and
// joins the accepted derived-index job.
func IndexEmbedBlockJobContext(ctx context.Context) error {
	r, err := captureIndexRepair(ctx)
	if err != nil {
		return err
	}
	_, err = task.AppendContext(context.WithoutCancel(ctx), task.DatabaseIndexEmbedBlock, func(life context.Context) error {
		work, cancel := context.WithTimeout(life, embedContentTimeout)
		defer cancel()
		return r.autoIndexEmbedBlocks(work)
	})
	return err
}

func (r *indexRepairContext) autoIndexEmbedBlocks(ctx context.Context) (err error) {
	indexEmbedBlockLock.Lock()
	defer indexEmbedBlockLock.Unlock()

	if err = r.guard(ctx); err != nil {
		return err
	}
	// Make the empty-content scan observe all earlier work from this owner.
	if err = r.queue.Flush(ctx); err != nil {
		return err
	}
	embedBlocks, err := indexdb.EmptyEmbedBlocks(ctx, r.main.DB)
	if err != nil {
		return err
	}
	dirty := false
	defer func() {
		if dirty {
			err = errors.Join(err, indexrepairDrain(ctx, r.queue))
		}
	}()
	for i, embedBlock := range embedBlocks {
		stmt := strings.TrimSpace(embedBlock.Markdown)
		stmt = strings.TrimPrefix(stmt, "{{")
		stmt = strings.TrimSuffix(stmt, "}}")
		stmt = html.UnescapeString(stmt)
		stmt = strings.ReplaceAll(stmt, editor.IALValEscNewLine, "\n")
		stmt = strings.TrimSpace(stmt)
		if strings.HasPrefix(stmt, "//!js") || !strings.Contains(strings.ToLower(stmt), "select") {
			continue
		}

		rows, queryErr := indexdb.SelectReadBlockRows(ctx, r.main.DB, stmt, 1, 102400)
		if queryErr != nil {
			return queryErr
		}
		var content strings.Builder
		for _, row := range rows {
			content.WriteString(row.Content)
		}
		value := content.String()
		if value == "" {
			value = "no query result"
		}
		if err = r.guard(ctx); err != nil {
			return err
		}
		if err = r.queue.UpdateBlockContent(ctx, &sql.Block{ID: embedBlock.ID, Content: value}); err != nil {
			return err
		}
		dirty = true
		if i >= 63 { // Preserve the original per-run upper bound.
			break
		}
	}
	return nil
}

func (r *indexRepairContext) updateEmbedBlockContent(ctx context.Context, embedBlockID string, markdown []string) error {
	if err := r.guard(ctx); err != nil {
		return err
	}
	rows, err := r.main.Blocks(ctx, []string{embedBlockID})
	if err != nil {
		return err
	}
	if len(rows) == 0 || rows[0] == nil {
		return ErrBlockNotFound
	}
	if rows[0].Type != "query_embed" {
		return errors.New("not query embed block")
	}
	var content strings.Builder
	for _, value := range markdown {
		content.WriteString(value)
	}
	value := content.String()
	if value == "" {
		value = "no query result"
	}
	if err = r.guard(ctx); err != nil {
		return err
	}
	return r.queue.UpdateBlockContent(ctx, &sql.Block{ID: embedBlockID, Content: value})
}

func scheduleEmbedBlockContent(ctx context.Context, embedBlockID string, blocks []*EmbedBlock) error {
	r, err := captureIndexRepair(ctx)
	if err != nil {
		return err
	}
	markdown := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block != nil && block.Block != nil {
			markdown = append(markdown, block.Block.Markdown)
		}
	}
	_, err = task.AppendContext(context.WithoutCancel(ctx), task.DatabaseIndexEmbedBlock, func(life context.Context) (runErr error) {
		work, cancel := context.WithTimeout(life, embedContentTimeout)
		defer cancel()
		defer func() { runErr = errors.Join(runErr, indexrepairDrain(work, r.queue)) }()
		return r.updateEmbedBlockContent(work, embedBlockID, markdown)
	})
	return err
}

func UpdateEmbedBlockContext(ctx context.Context, id, content string) error {
	r, err := captureIndexRepair(ctx)
	if err != nil {
		return err
	}
	if err = r.updateEmbedBlockContent(ctx, id, []string{content}); err != nil {
		return err
	}
	return r.queue.Flush(context.WithoutCancel(ctx))
}
