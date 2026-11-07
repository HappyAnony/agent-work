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

package model

import "context"

func CreateBox(name string) (string, error) { return CreateBoxContext(context.Background(), name) }

func RenameBox(id, name string) error { return RenameBoxContext(context.Background(), id, name) }

func RemoveBox(id string) error { return RemoveBoxContext(context.Background(), id) }
func Unmount(id string) {
	logOwnedTaskError("notebook unmount", UnmountContext(context.Background(), id))
}
func Mount(id string) (bool, error) { return MountContext(context.Background(), id) }

func IsUserGuide(boxID string) bool {
	return "20210808180117-czj9bvb" == boxID || "20210808180117-6v0mkxr" == boxID || "20211226090932-5lcq56f" == boxID || "20240530133126-axarxgx" == boxID
}
