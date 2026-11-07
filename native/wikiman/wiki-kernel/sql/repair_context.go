// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import "wiki-kernel/internal/indexdb"

// CaptureLegacyRepairQueries is used only by the compatibility owner while its
// lifecycle pins the current database. Subsequent reads never select global db.
func CaptureLegacyRepairQueries() indexdb.RepairQueries { return indexdb.RepairQueries{DB: db} }
