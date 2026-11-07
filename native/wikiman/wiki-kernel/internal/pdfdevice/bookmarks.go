// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package pdfdevice

import (
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"net/url"
	"sort"
	"strings"
)

const OutlineScheme = "pdf-outline"

type Heading struct {
	ID    string
	Level int
	Title string
	Found bool
}

func ReadOutlineLinks(pdf *pdfmodel.Context, guard func() error) (links []pdfmodel.LinkAnnotation, err error) {
	if pdf == nil || pdf.XRefTable == nil || guard == nil {
		return nil, errors.New("pdf_links_dependencies_unbound")
	}
	if err = guard(); err != nil {
		return nil, err
	}
	for page, annots := range pdf.PageAnnots {
		for _, item := range annots[pdfmodel.AnnLink].Map {
			if err = guard(); err != nil {
				return nil, err
			}
			link, ok := item.(pdfmodel.LinkAnnotation)
			if !ok {
				return nil, errors.New("pdf_outline_annotation_invalid")
			}
			if strings.HasPrefix(link.URI, OutlineScheme+"://") {
				link.Page = page
				links = append(links, link)
			}
		}
	}
	if err = guard(); err != nil {
		return nil, err
	}
	return links, nil
}
func BuildBookmarks(links []pdfmodel.LinkAnnotation, headings []Heading, lookup func(string) (string, bool, error), guard func() error) ([]*pdfcpu.Bookmark, error) {
	if lookup == nil || guard == nil {
		return nil, errors.New("pdf_bookmark_dependencies_unbound")
	}
	if err := guard(); err != nil {
		return nil, err
	}
	// Preserve page ordering, URL query decoding and original duplicate-title marker.
	ordered := append([]pdfmodel.LinkAnnotation(nil), links...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Page < ordered[j].Page })
	titles := map[string]bool{}
	byID := map[string]*pdfcpu.Bookmark{}
	for _, link := range ordered {
		if err := guard(); err != nil {
			return nil, err
		}
		id := link.URI[strings.LastIndex(link.URI, "/")+1:]
		title, ok, err := lookup(id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		title, _ = url.QueryUnescape(title)
		for titles[title] {
			title += "\x01"
		}
		titles[title] = true
		byID[id] = &pdfcpu.Bookmark{Title: title, PageFrom: link.Page, AbsPos: link.Rect.UR.Y}
	}
	seen := map[string]bool{}
	for _, h := range headings {
		if h.ID == "" || h.Level < 1 || h.Level > 6 || seen[h.ID] {
			return nil, errors.New("pdf_heading_invalid")
		}
		seen[h.ID] = true
	}
	var top, stack []*pdfcpu.Bookmark
	for _, heading := range headings {
		if err := guard(); err != nil {
			return nil, err
		}
		current := byID[heading.ID]
		if current == nil {
			continue
		} // A missing nested link must not dereference nil.
		for len(stack) > 0 && stack[len(stack)-1].Level >= heading.Level {
			stack = stack[:len(stack)-1]
		}
		current.Level = heading.Level
		if len(stack) == 0 {
			top = append(top, current)
		} else {
			parent := stack[len(stack)-1]
			current.Parent = parent
			parent.Kids = append(parent.Kids, current)
		}
		stack = append(stack, current)
	}
	if err := guard(); err != nil {
		return nil, err
	}
	return top, nil
}
func ApplyBookmarks(pdf *pdfmodel.Context, headings []Heading, lookup func(string) (string, bool, error), guard func() error) error {
	links, err := ReadOutlineLinks(pdf, guard)
	if err != nil {
		return err
	}
	bookmarks, err := BuildBookmarks(links, headings, lookup, guard)
	if err != nil {
		return err
	}
	if len(bookmarks) == 0 {
		return guard()
	}
	if err = guard(); err != nil {
		return err
	}
	if err = pdfcpu.AddBookmarks(pdf, bookmarks, true); err != nil {
		return err
	}
	return guard()
}
