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

// ToolBox: exported wrappers around unexported teardown helpers, needed so
// the workspace hot-switch pipeline (in this same package) can call them
// via a consistent API and so outside packages (e.g. api) can reason about
// them uniformly.
//
// See docs/wiki-workspace-hotswitch/design.md §3.2.2 Step 2.

package model

// CloseThemeWatchers exports the unexported closeThemeWatchers so the
// hot-switch pipeline can invoke it from other files/packages if needed.
// The resource owner removes all watches, closes the backend and waits for
// its event worker to exit. Stop is restartable; failures are logged.
func CloseThemeWatchers() {
	closeThemeWatchers()
}

// CloseSyncWebSocket exports the unexported closeSyncWebSocket so the
// hot-switch pipeline can terminate the cloud-sync WebSocket connection
// during Step 2 阶段 E.
//
// Note: this only affects the outbound sync connection used by the cloud
// sync feature, NOT the UI WebSocketServer which remains process-resident.
func CloseSyncWebSocket() {
	closeSyncWebSocket()
}

// ClearWorkspaceTemp exports the unexported clearWorkspaceTemp so the
// hot-switch pipeline can clean temp files of the outgoing workspace
// during Step 2 阶段 E.
func ClearWorkspaceTemp() {
	clearWorkspaceTemp()
}
