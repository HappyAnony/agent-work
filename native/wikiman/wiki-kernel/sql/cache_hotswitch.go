// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// ToolBox: cache cleanup helper for workspace hot-switch.
// See docs/wiki-workspace-hotswitch/design.md §3.2.2 Step 2 阶段 D.

package sql

// ClearBlockCache clears both the block cache (ristretto) and the
// def→refs cache (go-cache) used by the SQL layer. This is stronger than
// the existing ClearCache() (which only clears blockCache) and is intended
// for workspace hot-switch, where we must discard every cached reference
// to the previous workspace's data before booting the new workspace.
//
// Called during Step 2 阶段 D of the hot-switch pipeline.
func ClearBlockCache() {
	clearAllCaches()
}
