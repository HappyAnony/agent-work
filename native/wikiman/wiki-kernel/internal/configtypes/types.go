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

package configtypes

// StatusBar 底部状态栏配置。https://github.com/siyuan-note/siyuan/issues/16236
type StatusBar struct {
	MsgTaskDatabaseIndexCommitDisabled        bool `json:"msgTaskDatabaseIndexCommitDisabled"`
	MsgTaskHistoryDatabaseIndexCommitDisabled bool `json:"msgTaskHistoryDatabaseIndexCommitDisabled"`
	MsgTaskAssetDatabaseIndexCommitDisabled   bool `json:"msgTaskAssetDatabaseIndexCommitDisabled"`
	MsgTaskHistoryGenerateFileDisabled        bool `json:"msgTaskHistoryGenerateFileDisabled"`
}

const (
	SortModeNameASC         = iota // 0：文件名字母升序
	SortModeNameDESC               // 1：文件名字母降序
	SortModeUpdatedASC             // 2：文件更新时间升序
	SortModeUpdatedDESC            // 3：文件更新时间降序
	SortModeAlphanumASC            // 4：文件名自然数升序
	SortModeAlphanumDESC           // 5：文件名自然数降序
	SortModeCustom                 // 6：自定义排序
	SortModeRefCountASC            // 7：引用数升序
	SortModeRefCountDESC           // 8：引用数降序
	SortModeCreatedASC             // 9：文件创建时间升序
	SortModeCreatedDESC            // 10：文件创建时间降序
	SortModeSizeASC                // 11：文件大小升序
	SortModeSizeDESC               // 12：文件大小降序
	SortModeSubDocCountASC         // 13：子文档数升序
	SortModeSubDocCountDESC        // 14：子文档数降序
	SortModeFileTree               // 15：使用文档树排序规则

	SortModeUnassigned = 256 // 256：未指定排序规则，按照笔记本优先于文档树获取排序规则
)
