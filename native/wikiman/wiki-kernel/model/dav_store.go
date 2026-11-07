// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package model

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/emersion/go-webdav/caldav"
	"github.com/emersion/go-webdav/carddav"
)

var (
	errDAVOwnerUnbound = errors.New("dav_owner_unbound")
	errDAVRootInvalid  = errors.New("dav_root_invalid")
	errDAVPathInvalid  = errors.New("dav_path_invalid")
)

type DAVStore struct {
	dataRoot  string
	calendars *Calendars
	contacts  *Contacts
}

func NewDAVStore(dataRoot string) (*DAVStore, error) {
	dataRoot = filepath.Clean(strings.TrimSpace(dataRoot))
	if dataRoot == "." || !filepath.IsAbs(dataRoot) {
		return nil, errDAVRootInvalid
	}
	store := &DAVStore{dataRoot: dataRoot}
	store.calendars = newCalendars(dataRoot)
	store.contacts = newContacts(dataRoot)
	return store, nil
}

func (s *DAVStore) CalDavBackend() *CalDavBackend {
	if s == nil {
		return &CalDavBackend{}
	}
	return &CalDavBackend{calendars: s.calendars}
}

func (s *DAVStore) CardDavBackend() *CardDavBackend {
	if s == nil {
		return &CardDavBackend{}
	}
	return &CardDavBackend{contacts: s.contacts}
}

func davStoragePath(dataRoot, davPath string) (string, error) {
	if dataRoot == "" || !filepath.IsAbs(dataRoot) {
		return "", errDAVRootInvalid
	}
	davPath = filepath.ToSlash(strings.TrimSpace(davPath))
	if davPath == "" {
		return "", errDAVPathInvalid
	}
	relative := strings.TrimPrefix(davPath, "/")
	local := filepath.FromSlash(relative)
	if !filepath.IsLocal(local) {
		return "", errDAVPathInvalid
	}
	return filepath.Join(dataRoot, "storage", local), nil
}

func newCalendars(dataRoot string) *Calendars {
	return &Calendars{
		dataRoot:          dataRoot,
		calendarsMetaData: []*caldav.Calendar{},
	}
}

func newContacts(dataRoot string) *Contacts {
	return &Contacts{
		dataRoot:      dataRoot,
		booksMetaData: []*carddav.AddressBook{},
	}
}
