// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

import (
	"github.com/open-spaced-repetition/go-fsrs/v3"
	"time"
)

// Block 描述了内容块。
type Block struct {
	Box      string            `json:"box"`
	Path     string            `json:"path"`
	HPath    string            `json:"hPath"`
	ID       string            `json:"id"`
	RootID   string            `json:"rootID"`
	ParentID string            `json:"parentID"`
	Name     string            `json:"name"`
	Alias    string            `json:"alias"`
	Memo     string            `json:"memo"`
	Tag      string            `json:"tag"`
	Content  string            `json:"content"`
	FContent string            `json:"fcontent"`
	Markdown string            `json:"markdown"`
	Folded   bool              `json:"folded"`
	Type     string            `json:"type"`
	SubType  string            `json:"subType"`
	RefText  string            `json:"refText"`
	Defs     []*Block          `json:"-"`    // 当前块引用了这些块，避免序列化 JSON 时产生循环引用
	Refs     []*Block          `json:"refs"` // 当前块被这些块引用
	DefID    string            `json:"defID"`
	DefPath  string            `json:"defPath"`
	IAL      map[string]string `json:"ial"`
	Children []*Block          `json:"children"`
	Depth    int               `json:"depth"`
	Count    int               `json:"count"`
	RefCount int               `json:"refCount"`
	Sort     int               `json:"sort"`
	Created  string            `json:"created"`
	Updated  string            `json:"updated"`

	RiffCardID string    `json:"riffCardID"`
	RiffCard   *RiffCard `json:"riffCard"`
}

type RiffCard struct {
	Due        time.Time  `json:"due"`
	Reps       uint64     `json:"reps"`
	Lapses     uint64     `json:"lapses"`
	State      fsrs.State `json:"state"`
	LastReview time.Time  `json:"lastReview"`
}

func (block *Block) IsContainerBlock() bool {
	switch block.Type {
	case "NodeDocument", "NodeBlockquote", "NodeList", "NodeListItem", "NodeSuperBlock":
		return true
	}
	return false
}

func (block *Block) IsDoc() bool {
	return "NodeDocument" == block.Type
}
