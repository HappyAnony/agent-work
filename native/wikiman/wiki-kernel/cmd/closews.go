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

package cmd

type closews struct {
	*BaseCmd
}

func (cmd *closews) Exec() {
	// Close the captured connection, never a replacement or another app's ID.
	_ = cmd.session.CloseWithMsg([]byte("  close websocket"))
	cmd.Push()
}

func (cmd *closews) Name() string {
	return "closews"
}

func (cmd *closews) IsRead() bool {
	return true
}
