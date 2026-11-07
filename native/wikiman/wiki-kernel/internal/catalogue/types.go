// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package catalogue

type DisplayName struct {
	Default string `json:"default"`
	ArSA    string `json:"ar_SA"`
	DeDE    string `json:"de_DE"`
	EnUS    string `json:"en_US"`
	EsES    string `json:"es_ES"`
	FrFR    string `json:"fr_FR"`
	HeIL    string `json:"he_IL"`
	ItIT    string `json:"it_IT"`
	JaJP    string `json:"ja_JP"`
	PlPL    string `json:"pl_PL"`
	PtBR    string `json:"pt_BR"`
	RuRU    string `json:"ru_RU"`
	ZhCHT   string `json:"zh_CHT"`
	ZhCN    string `json:"zh_CN"`
}

type Description struct {
	Default string `json:"default"`
	ArSA    string `json:"ar_SA"`
	DeDE    string `json:"de_DE"`
	EnUS    string `json:"en_US"`
	EsES    string `json:"es_ES"`
	FrFR    string `json:"fr_FR"`
	HeIL    string `json:"he_IL"`
	ItIT    string `json:"it_IT"`
	JaJP    string `json:"ja_JP"`
	PlPL    string `json:"pl_PL"`
	PtBR    string `json:"pt_BR"`
	RuRU    string `json:"ru_RU"`
	ZhCHT   string `json:"zh_CHT"`
	ZhCN    string `json:"zh_CN"`
}

type Readme struct {
	Default string `json:"default"`
	ArSA    string `json:"ar_SA"`
	DeDE    string `json:"de_DE"`
	EnUS    string `json:"en_US"`
	EsES    string `json:"es_ES"`
	FrFR    string `json:"fr_FR"`
	HeIL    string `json:"he_IL"`
	ItIT    string `json:"it_IT"`
	JaJP    string `json:"ja_JP"`
	PlPL    string `json:"pl_PL"`
	PtBR    string `json:"pt_BR"`
	RuRU    string `json:"ru_RU"`
	ZhCHT   string `json:"zh_CHT"`
	ZhCN    string `json:"zh_CN"`
}

type Funding struct {
	OpenCollective string   `json:"openCollective"`
	Patreon        string   `json:"patreon"`
	GitHub         string   `json:"github"`
	Custom         []string `json:"custom"`
}

type Package struct {
	Author            string       `json:"author"`
	URL               string       `json:"url"`
	Version           string       `json:"version"`
	MinAppVersion     string       `json:"minAppVersion"`
	DisabledInPublish bool         `json:"disabledInPublish"`
	Backends          []string     `json:"backends"`
	Frontends         []string     `json:"frontends"`
	DisplayName       *DisplayName `json:"displayName"`
	Description       *Description `json:"description"`
	Readme            *Readme      `json:"readme"`
	Funding           *Funding     `json:"funding"`
	Keywords          []string     `json:"keywords"`

	PreferredFunding string `json:"preferredFunding"`
	PreferredName    string `json:"preferredName"`
	PreferredDesc    string `json:"preferredDesc"`
	PreferredReadme  string `json:"preferredReadme"`

	Name            string `json:"name"`
	RepoURL         string `json:"repoURL"`
	RepoHash        string `json:"repoHash"`
	PreviewURL      string `json:"previewURL"`
	PreviewURLThumb string `json:"previewURLThumb"`
	IconURL         string `json:"iconURL"`

	Installed    bool   `json:"installed"`
	Outdated     bool   `json:"outdated"`
	Current      bool   `json:"current"`
	Updated      string `json:"updated"`
	Stars        int    `json:"stars"`
	OpenIssues   int    `json:"openIssues"`
	Size         int64  `json:"size"`
	HSize        string `json:"hSize"`
	InstallSize  int64  `json:"installSize"`
	HInstallSize string `json:"hInstallSize"`
	HInstallDate string `json:"hInstallDate"`
	HUpdated     string `json:"hUpdated"`
	Downloads    int    `json:"downloads"`

	Incompatible bool `json:"incompatible"`
}

type StagePackage struct {
	Author      string       `json:"author"`
	URL         string       `json:"url"`
	Version     string       `json:"version"`
	Description *Description `json:"description"`
	Readme      *Readme      `json:"readme"`
	I18N        []string     `json:"i18n"`
	Funding     *Funding     `json:"funding"`
}

type StageRepo struct {
	URL         string `json:"url"`
	Updated     string `json:"updated"`
	Stars       int    `json:"stars"`
	OpenIssues  int    `json:"openIssues"`
	Size        int64  `json:"size"`
	InstallSize int64  `json:"installSize"`

	Package *StagePackage `json:"package"`
}

type StageIndex struct {
	Repos []*StageRepo `json:"repos"`
}

type Plugin struct {
	*Package
	Enabled bool `json:"enabled"`
}
type Widget struct {
	*Package
}
type Icon struct {
	*Package
}
type Theme struct {
	*Package

	Modes []string `json:"modes"`
}
type Template struct {
	*Package
}
