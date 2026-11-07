// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package conf

// Masked preserves the complete original serialized shape without persistence
// authority or the non-serialized account object. Every returned field is owned.
func (data AppData) Masked() (AppData, error) {
	out, e := data.Clone()
	if e != nil {
		return AppData{}, e
	}
	out.User = nil
	out.UserData = ""
	if out.AccessAuthCode != "" {
		out.AccessAuthCode = "*******"
	}
	return out, nil
}

// HideSecrets is the original non-admin projection, including its field shapes.
// It acts only on an already detached snapshot and tolerates missing System.
func (data *AppData) HideSecrets() {
	data.AI = &AI{}
	data.Api = &API{}
	data.Flashcard = &Flashcard{}
	data.LocalIPs = []string{}
	data.Publish = &Publish{}
	data.Repo = &Repo{}
	data.Sync = &Sync{}
	if data.System == nil {
		data.System = &System{}
	}
	data.System.AppDir = ""
	data.System.ConfDir = ""
	data.System.DataDir = ""
	data.System.HomeDir = ""
	data.System.WorkspaceDir = ""
	data.System.Name = ""
	data.System.NetworkProxy = &NetworkProxy{}
}
func (data AppData) IsSubscriber() bool {
	u := data.User
	return u != nil && (u.UserSiYuanProExpireTime == -1 || u.UserSiYuanProExpireTime > 0) && u.UserSiYuanSubscriptionStatus == 0
}
func (data AppData) IsPaidUser() bool {
	return data.IsSubscriber() || data.User != nil && data.User.UserSiYuanOneTimePayStatus == 1
}

type PresentationOptions struct {
	Restricted, Publish bool
	DisabledSyncText    string
}

func (data AppData) Presentation(q PresentationOptions) (AppData, error) {
	out, e := data.Masked()
	if e != nil {
		return AppData{}, e
	}
	if out.Sync != nil && (!out.Sync.Enabled || out.Sync.Provider == 0 && !data.IsSubscriber()) {
		out.Sync.Stat = q.DisabledSyncText
	}
	if q.Publish {
		out.ReadOnly = true
	}
	if q.Restricted {
		out.HideSecrets()
	}
	return out, nil
}
