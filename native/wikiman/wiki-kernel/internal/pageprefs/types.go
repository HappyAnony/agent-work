// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package pageprefs

type RecentDoc struct {
	RootID   string `json:"rootID"`
	Icon     string `json:"icon"`
	Title    string `json:"title"`
	ViewedAt int64  `json:"viewedAt"` // 浏览时间字段
	ClosedAt int64  `json:"closedAt"` // 关闭时间字段
	OpenAt   int64  `json:"openAt"`   // 文档第一次从文档树加载到页签的时间
}

type OutlineDoc struct {
	DocID string                 `json:"docID"`
	Data  map[string]interface{} `json:"data"`
}

type Criterion struct {
	Name         string                 `json:"name"`
	Sort         int                    `json:"sort"`       // 0：按块类型（默认），1：按创建时间升序，2：按创建时间降序，3：按更新时间升序，4：按更新时间降序，5：按内容顺序（仅在按文档分组时）
	Group        int                    `json:"group"`      // 0：不分组，1：按文档分组
	HasReplace   bool                   `json:"hasReplace"` // 是否有替换
	Method       int                    `json:"method"`     // 0：文本，1：查询语法，2：SQL，3：正则表达式
	HPath        string                 `json:"hPath"`
	IDPath       []string               `json:"idPath"`
	K            string                 `json:"k"`            // 搜索关键字
	R            string                 `json:"r"`            // 替换关键字
	Types        *CriterionTypes        `json:"types"`        // 类型过滤选项
	ReplaceTypes *CriterionReplaceTypes `json:"replaceTypes"` // 替换类型过滤选项
}

type CriterionTypes struct {
	MathBlock     bool `json:"mathBlock"`
	Table         bool `json:"table"`
	Blockquote    bool `json:"blockquote"`
	SuperBlock    bool `json:"superBlock"`
	Paragraph     bool `json:"paragraph"`
	Document      bool `json:"document"`
	Heading       bool `json:"heading"`
	List          bool `json:"list"`
	ListItem      bool `json:"listItem"`
	CodeBlock     bool `json:"codeBlock"`
	HtmlBlock     bool `json:"htmlBlock"`
	EmbedBlock    bool `json:"embedBlock"`
	DatabaseBlock bool `json:"databaseBlock"`
	AudioBlock    bool `json:"audioBlock"`
	VideoBlock    bool `json:"videoBlock"`
	IFrameBlock   bool `json:"iframeBlock"`
	WidgetBlock   bool `json:"widgetBlock"`
}

type CriterionReplaceTypes struct {
	Text              bool `json:"text"`
	ImgText           bool `json:"imgText"`
	ImgTitle          bool `json:"imgTitle"`
	ImgSrc            bool `json:"imgSrc"`
	AText             bool `json:"aText"`
	ATitle            bool `json:"aTitle"`
	AHref             bool `json:"aHref"`
	Code              bool `json:"code"`
	Em                bool `json:"em"`
	Strong            bool `json:"strong"`
	InlineMath        bool `json:"inlineMath"`
	InlineMemo        bool `json:"inlineMemo"`
	BlockRef          bool `json:"blockRef"`
	FileAnnotationRef bool `json:"fileAnnotationRef"`
	Kbd               bool `json:"kbd"`
	Mark              bool `json:"mark"`
	S                 bool `json:"s"`
	Sub               bool `json:"sub"`
	Sup               bool `json:"sup"`
	Tag               bool `json:"tag"`
	U                 bool `json:"u"`
	DocTitle          bool `json:"docTitle"`
	CodeBlock         bool `json:"codeBlock"`
	MathBlock         bool `json:"mathBlock"`
	HtmlBlock         bool `json:"htmlBlock"`
}
