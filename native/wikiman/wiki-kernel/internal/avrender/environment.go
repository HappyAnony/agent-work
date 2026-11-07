// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"errors"
	"github.com/88250/lute/parse"
	av "wiki-kernel/internal/avmodel"
)

var ErrEnvironmentUnbound = errors.New("AV table rendering dependencies are not bound")
var ErrUnsupported = errors.New("AV rendering feature is not context-bound yet")
var ErrShape = errors.New("invalid AV rendering shape")
var ErrLimit = errors.New("AV rendering limit exceeded")

type Environment struct {
	Trees              func([]string) (map[string]*parse.Tree, error)
	Cover              CoverServices
	RemoveMissingField func(*av.AttributeView, *av.View, string) error
	FilterItems        func(map[string][]*av.KeyValues) error
	BlockAttrs         func([]string) (map[string]map[string]string, error)
	AutoValues         func(*av.AttributeView, av.Collection, map[string]map[string]string) error
	Templates          func(*av.AttributeView, *av.View, av.Collection, map[string]map[string]string) error
}

func (e Environment) validate() error {
	if e.RemoveMissingField == nil || e.FilterItems == nil || e.BlockAttrs == nil || e.AutoValues == nil || e.Templates == nil {
		return ErrEnvironmentUnbound
	}
	return nil
}
