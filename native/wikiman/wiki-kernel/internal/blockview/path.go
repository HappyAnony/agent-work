// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

type Path struct {
	ID       string   `json:"id"`                 // 块 ID
	Box      string   `json:"box"`                // 块 Box
	Name     string   `json:"name"`               // 当前路径
	HPath    string   `json:"hPath"`              // 人类可读路径
	Type     string   `json:"type"`               // "path"
	NodeType string   `json:"nodeType"`           // 节点类型
	SubType  string   `json:"subType"`            // 节点子类型
	Blocks   []*Block `json:"blocks,omitempty"`   // 子块节点
	Children []*Path  `json:"children,omitempty"` // 子路径节点
	Depth    int      `json:"depth"`              // 层级深度
	Count    int      `json:"count"`              // 子块计数
	Folded   bool     `json:"folded"`             // 是否折叠

	Updated string `json:"updated"` // 更新时间
	Created string `json:"created"` // 创建时间
}

type BlockPath struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Type     string       `json:"type"`
	SubType  string       `json:"subType"`
	Children []*BlockPath `json:"children"`
}
