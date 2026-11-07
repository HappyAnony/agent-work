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

package conf

// AppData is the complete original serialized AppConf data contract.
// No process globals, filesystem IO, token generation or model workers are used.
type AppData struct {
	LogLevel       string      `json:"logLevel"`       // 日志级别：off, trace, debug, info, warn, error, fatal
	Appearance     *Appearance `json:"appearance"`     // 外观
	Langs          []*Lang     `json:"langs"`          // 界面语言列表
	Lang           string      `json:"lang"`           // 选择的界面语言，同 Appearance.Lang
	FileTree       *FileTree   `json:"fileTree"`       // 文档面板
	Tag            *Tag        `json:"tag"`            // 标签面板
	Editor         *Editor     `json:"editor"`         // 编辑器配置
	Export         *Export     `json:"export"`         // 导出配置
	Graph          *Graph      `json:"graph"`          // 关系图配置
	UILayout       *UILayout   `json:"uiLayout"`       // 界面布局。不要直接使用，使用 GetUILayout() 和 SetUILayout() 方法
	UserData       string      `json:"userData"`       // 社区用户信息，对 User 加密存储
	User           *User       `json:"-"`              // 社区用户内存结构，不持久化。不要直接使用，使用 GetUser() 和 SetUser() 方法
	Account        *Account    `json:"account"`        // 帐号配置
	ReadOnly       bool        `json:"readonly"`       // 是否是以只读模式运行
	LocalIPs       []string    `json:"localIPs"`       // 本地 IP 列表
	AccessAuthCode string      `json:"accessAuthCode"` // 访问授权码
	System         *System     `json:"system"`         // 系统配置
	Keymap         *Keymap     `json:"keymap"`         // 快捷键配置
	Sync           *Sync       `json:"sync"`           // 同步配置
	Search         *Search     `json:"search"`         // 搜索配置
	Flashcard      *Flashcard  `json:"flashcard"`      // 闪卡配置
	AI             *AI         `json:"ai"`             // 人工智能配置
	Bazaar         *Bazaar     `json:"bazaar"`         // 集市配置
	Stat           *Stat       `json:"stat"`           // 统计
	Api            *API        `json:"api"`            // API
	Repo           *Repo       `json:"repo"`           // 数据仓库
	Publish        *Publish    `json:"publish"`        // 发布服务
	OpenHelp       bool        `json:"openHelp"`       // 启动后是否需要打开用户指南
	ShowChangelog  bool        `json:"showChangelog"`  // 是否显示版本更新日志
	CloudRegion    int         `json:"cloudRegion"`    // 云端区域，0：中国大陆，1：北美
	Snippet        *Snpt       `json:"snippet"`        // 代码片段
	DataIndexState int         `json:"dataIndexState"` // 数据索引状态，0：已索引，1：未索引
}
