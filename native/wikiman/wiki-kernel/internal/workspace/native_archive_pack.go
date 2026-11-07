// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"io"
	"wiki-kernel/internal/archivepack"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

// WriteNativeDataArchive borrows the selected owner's source and joins all IO.
// Consumers publish a resulting file only after this method returns success.
func (l *Lease) WriteNativeDataArchive(ctx context.Context, prefix string, dst io.Writer) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		return archivepack.WriteWithIO(ctx, indexrepair.Source{Data: l.workspace.data}, prefix, dst, ctx.Err, func(w io.Writer, r io.Reader) error { return archivepack.CopyContext(ctx, w, r) }, archivepack.WalkIO{})
	})
}
