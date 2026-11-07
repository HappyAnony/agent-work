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

// ToolBox: cleanup helpers for workspace hot-switch.
// See docs/wiki-workspace-hotswitch/design.md §3.2.2 Step 2 阶段 D.

package treenode

// ClearDynamicRefTexts empties the DynamicRefTexts sync.Map.
// sync.Map has no built-in clear method, so we iterate and delete.
// Called during Step 2 阶段 D of the hot-switch pipeline.
func ClearDynamicRefTexts() {
	DynamicRefTexts.Range(func(key, _ interface{}) bool {
		DynamicRefTexts.Delete(key)
		return true
	})
}
