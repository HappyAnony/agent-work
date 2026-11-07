// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"wiki-kernel/internal/extensionprefs"
)

type Petal = extensionprefs.Petal

func LoadPetals(frontend string, publish bool) []*Petal {
	v, e := LoadPetalsContext(context.Background(), frontend, publish)
	if e != nil {
		logOwnedTaskError("plugin load", e)
		return []*Petal{}
	}
	return v
}
func SetPetalEnabled(name string, enabled bool, frontend string) (*Petal, error) {
	return SetPetalEnabledContext(context.Background(), name, enabled, frontend, "", false)
}
func getPetals() []*Petal {
	v, e := GetPetalsContext(context.Background())
	if e != nil {
		logOwnedTaskError("plugin metadata", e)
		return []*Petal{}
	}
	return v
}
func savePetals(v []*Petal) {
	logOwnedTaskError("plugin metadata save", SavePetalsContext(context.Background(), v))
}
func getPetalByName(name string, petals []*Petal) *Petal {
	for _, v := range petals {
		if v.Name == name {
			return v
		}
	}
	return nil
}
