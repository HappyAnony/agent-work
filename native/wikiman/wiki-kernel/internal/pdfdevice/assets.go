// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package pdfdevice

import (
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"io/fs"
	"net/url"
	"strings"
	"time"
)

var ErrAssetPath = errors.New("pdf_asset_path_invalid")

type AssetURL struct{ Path, URI string }
type AssetFile struct{ Path, Name string }
type AssetServices struct {
	Guard                         func() error
	Classify                      func(string) bool
	Normalize                     func(string) (AssetURL, error)
	Resolve                       func(string) (AssetFile, bool, error)
	Embed                         func(*pdfmodel.Context, AssetFile) (*types.IndirectRef, error)
	Now                           time.Time
	DecodeOther, SkipEmbedFailure bool
}

func ReadAssetLinks(pdf *pdfmodel.Context, classify func(string) bool, guard func() error) (assets, others []pdfmodel.LinkAnnotation, err error) {
	if pdf == nil || pdf.XRefTable == nil || classify == nil || guard == nil {
		return nil, nil, errors.New("pdf_assets_dependencies_unbound")
	}
	for page, annotations := range pdf.PageAnnots {
		if err = guard(); err != nil {
			return nil, nil, err
		}
		for _, value := range annotations[pdfmodel.AnnLink].Map {
			if err = guard(); err != nil {
				return nil, nil, err
			}
			link, ok := value.(pdfmodel.LinkAnnotation)
			if !ok {
				return nil, nil, errors.New("pdf_asset_annotation_invalid")
			}
			link.Page = page
			if classify(link.URI) {
				assets = append(assets, link)
			} else {
				others = append(others, link)
			}
		}
	}
	if err = guard(); err != nil {
		return nil, nil, err
	}
	return
}
func NormalizeAssetURL(value, origin string) (AssetURL, error) {
	u, err := url.Parse(value)
	if err != nil {
		return AssetURL{}, err
	}
	if u.User != nil || u.Scheme+"://"+u.Host != origin {
		return AssetURL{}, ErrAssetPath
	}
	encoded := strings.TrimPrefix(u.EscapedPath(), "/")
	encoded = strings.TrimPrefix(encoded, "export/temp/")
	decoded, err := url.PathUnescape(encoded)
	if err != nil {
		return AssetURL{}, err
	}
	if !strings.HasPrefix(decoded, "assets/") || !fs.ValidPath(strings.TrimSuffix(decoded, "/")) || strings.Contains(decoded, "\\") {
		return AssetURL{}, ErrAssetPath
	}
	uri := encoded
	if u.Fragment != "" {
		uri += "#" + u.EscapedFragment()
	}
	return AssetURL{decoded, uri}, nil
}
func ApplyAssetLinks(pdf *pdfmodel.Context, destinations []string, removeAssets bool, s AssetServices) error {
	if pdf == nil || pdf.XRefTable == nil {
		return errors.New("pdf_assets_dependencies_unbound")
	}
	if s.Guard == nil || s.Classify == nil || s.Normalize == nil || s.Resolve == nil || s.Embed == nil {
		return errors.New("pdf_assets_dependencies_unbound")
	}
	if err := s.Guard(); err != nil {
		return err
	}
	hasAssets := false
	for _, destination := range destinations {
		if _, exists, err := s.Resolve(destination); err != nil {
			return err
		} else if exists {
			hasAssets = true
		}
	}
	if !hasAssets {
		return s.Guard()
	}
	assets, others, err := ReadAssetLinks(pdf, s.Classify, s.Guard)
	if err != nil {
		return err
	}
	if len(assets) == 0 {
		return s.Guard()
	}
	type planned struct {
		link   pdfmodel.LinkAnnotation
		file   AssetFile
		exists bool
	}
	plans := make([]planned, 0, len(assets))
	for _, link := range assets {
		if err = s.Guard(); err != nil {
			return err
		}
		relative, e := s.Normalize(link.URI)
		if e != nil {
			return e
		}
		file, exists, e := s.Resolve(relative.Path)
		if e != nil {
			return e
		}
		link.URI = relative.URI
		plans = append(plans, planned{link, file, exists})
	}
	if err = s.Guard(); err != nil {
		return err
	}
	// Replace links only. Existing notes/highlights are unrelated to asset links.
	if _, err = pdfcpu.RemoveAnnotations(pdf, nil, []string{"Link"}, nil, false); err != nil {
		return err
	}
	links := map[int][]pdfmodel.AnnotationRenderer{}
	attachments := map[int][]*types.IndirectRef{}
	for _, link := range others {
		if s.DecodeOther {
			link.URI, _ = url.PathUnescape(link.URI)
		}
		links[link.Page] = append(links[link.Page], link)
	}
	for _, p := range plans {
		if err = s.Guard(); err != nil {
			return err
		}
		if !removeAssets {
			links[p.link.Page] = append(links[p.link.Page], p.link)
			continue
		}
		if !p.exists {
			continue
		} // Preserve original missing-source omission in embed mode.
		stream, e := s.Embed(pdf, p.file)
		if e != nil {
			if s.SkipEmbedFailure {
				continue
			}
			return e
		}
		if stream == nil {
			return errors.New("pdf_asset_stream_missing")
		}
		spec, e := pdf.XRefTable.NewFileSpecDict(p.file.Name, p.file.Name, "attached by wiki", *stream)
		if e != nil {
			return e
		}
		specRef, e := pdf.IndRefForNewObject(spec)
		if e != nil {
			return e
		}
		ref, e := NewAssetAnnotation(pdf, p.link, *specRef, s.Now)
		if e != nil {
			return e
		}
		attachments[p.link.Page] = append(attachments[p.link.Page], ref)
	}
	if len(links) > 0 {
		if _, err = pdfcpu.AddAnnotationsMap(pdf, links, false); err != nil {
			return err
		}
	}
	for page, refs := range attachments {
		if err = s.Guard(); err != nil {
			return err
		}
		ref, e := pdf.PageDictIndRef(page)
		if e != nil {
			return e
		}
		dict, e := pdf.DereferenceDict(*ref)
		if e != nil {
			return e
		}
		var array types.Array
		if object, exists := dict.Find("Annots"); exists {
			array, e = pdf.DereferenceArray(object)
			if e != nil {
				return e
			}
		}
		for _, ref := range refs {
			array = append(array, *ref)
		}
		dict.Update("Annots", array)
		pdf.EnsureVersionForWriting()
	}
	return s.Guard()
}
func NewAssetAnnotation(pdf *pdfmodel.Context, link pdfmodel.LinkAnnotation, file types.IndirectRef, now time.Time) (*types.IndirectRef, error) {
	rect := link.Rect
	date := types.StringLiteral(types.DateString(now))
	dict := types.Dict{"Type": types.Name("Annot"), "Subtype": types.Name("FileAttachment"), "Contents": types.StringLiteral(""), "Rect": types.RectForWidthAndHeight(rect.LL.X+rect.Width(), rect.LL.Y+rect.Height()/2, rect.Height()/2, rect.Height()/2).Array(), "P": link.P, "M": date, "F": types.Integer(0), "Border": types.NewIntegerArray(0, 0, 1), "C": types.NewNumberArray(0.5, 0, 0.5), "CA": types.Float(0.95), "CreationDate": date, "Name": types.Name("FileAttachment"), "FS": file, "NM": types.StringLiteral("")}
	return pdf.IndRefForNewObject(dict)
}
