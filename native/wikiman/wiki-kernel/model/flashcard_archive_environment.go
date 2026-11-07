package model

import (
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/flashcard"
)

func (tx *Transaction) importFlashcardArchive(files map[string][]byte, ids map[string]string, trees []*parse.Tree) (out flashcard.ArchivePlan, err error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		e := tx.environment.flashcard
		if e == nil || e.gate == nil || e.gate.token == nil || tx.environment.ctx == nil {
			return out, flashcard.ErrDependency
		}
		err = e.gate.with(tx.environment.ctx, func() error {
			var e2 error
			out, e2 = flashcard.ExecuteImport(tx.environment.ctx, files, ids, trees, e.archive)
			return e2
		})
		return
	}
	return runLegacyFlashcardArchive(legacyAVBindingForTransaction(tx), files, ids, trees, "", false)
}
func (tx *Transaction) exportFlashcardArchive(trees []*parse.Tree, folder string) (out flashcard.ArchivePlan, err error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		e := tx.environment.flashcard
		if e == nil || e.gate == nil || e.gate.token == nil || tx.environment.ctx == nil {
			return out, flashcard.ErrDependency
		}
		err = e.gate.with(tx.environment.ctx, func() error {
			var e2 error
			out, e2 = flashcard.ExecuteExport(tx.environment.ctx, trees, e.archive)
			return e2
		})
		return
	}
	return runLegacyFlashcardArchive(legacyAVBindingForTransaction(tx), nil, nil, trees, folder, true)
}
