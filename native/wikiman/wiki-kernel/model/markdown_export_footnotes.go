// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute/parse"
)

// loadTreeWithCache keys by the requested ID, including a non-root definition
// block. Preserve that behavior: do not substitute an already transformed root
// candidate when the original helper would have loaded a separate tree.
func exportFootnoteLoad(id string, cache map[string]*parse.Tree, ports []*exportReferenceRenderer) (*parse.Tree, error) {
	if len(ports) == 0 {
		return loadTreeWithCache(id, cache)
	}
	p := ports[0]
	if err := p.check(); err != nil {
		return nil, err
	}
	bt := exportRenderLookup(id, ports)
	if bt == nil {
		if p.err == nil {
			p.err = ErrBlockNotFound
		}
		return nil, p.check()
	}
	if tree := cache[id]; tree != nil {
		if tree.ID != bt.RootID || tree.Box != bt.BoxID || tree.Path != bt.Path {
			exportRenderFailure(ports, errors.New("export_footnote_identity_changed"))
			return nil, p.check()
		}
		return tree, p.check()
	}
	tree, err := exportEmbedLoad(id, ports)
	if err != nil {
		return nil, err
	}
	cache[id] = tree
	return tree, p.check()
}
