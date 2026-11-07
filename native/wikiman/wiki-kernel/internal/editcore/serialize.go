// SPDX-License-Identifier: AGPL-3.0-or-later
package editcore

import "wiki-kernel/internal/docast"

var ErrOpaqueEdit = docast.ErrOpaqueEdit

func SerializeDocument(before [][]byte, next []byte, preservation ...docast.DocumentPreservation) ([]byte, error) {
	return docast.SerializeDocument(before, next, preservation...)
}
func SerializeAttributeView(before, next []byte) ([]byte, error) {
	return docast.SerializeAttributeView(before, next)
}
