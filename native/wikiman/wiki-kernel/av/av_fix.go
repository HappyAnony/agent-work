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

package av

import (
	"github.com/88250/lute/ast"
	"github.com/siyuan-note/logging"
	"time"
	"wiki-kernel/internal/avmodel"
)

const CurrentSpec = avmodel.CurrentSpec

func UpgradeSpec(a *AttributeView) {
	if a.Spec >= CurrentSpec {
		return
	}
	if e := avmodel.UpgradeInMemory(a, avmodel.UpgradeEnvironment{Now: time.Now, NewID: ast.NewNodeID, Warning: func(e error) { logging.LogWarnf("%s", e) }}); e != nil {
		logging.LogWarnf("upgrade AV: %s", e)
	}
}
