// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

func (store *Store) check() error {
	if store.admission != nil {
		if e := store.admission(); e != nil {
			return e
		}
	}
	if store.operation != nil {
		return store.operation.Err()
	}
	return nil
}
func (store *Store) writing() error {
	if e := store.check(); e != nil {
		return e
	}
	if store.modified != nil {
		store.modified()
	}
	return nil
}
