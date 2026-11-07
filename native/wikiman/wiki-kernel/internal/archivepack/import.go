// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package archivepack

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/textprefs"
)

var ErrArchive = errors.New("invalid_document_archive")

// ValidateMembers is shared with the original archive extractor.
func ValidateMembers(files []*zip.File) error {
	seen := map[string]bool{}
	for _, f := range files {
		name := strings.TrimSuffix(f.Name, "/")
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\\x00") || (!f.Mode().IsRegular() && !f.FileInfo().IsDir()) || seen[name] {
			return ErrArchive
		}
		seen[name] = true
	}
	return nil
}

// ReadDocumentArchive uses the original single-directory document archive
// format. It expands bounded entries into a private candidate in memory.
func ReadDocumentArchive(ctx context.Context, raw []byte) (map[string][]byte, error) {
	return readArchive(ctx, raw, true)
}

// ReadMarkdownArchive accepts regular ZIP files with or without an enclosing directory.
func ReadMarkdownArchive(ctx context.Context, raw []byte) (map[string][]byte, error) {
	return readArchive(ctx, raw, false)
}

func readArchive(ctx context.Context, raw []byte, document bool) (map[string][]byte, error) {
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || len(z.File) == 0 || len(z.File) > 5000 {
		return nil, ErrArchive
	}
	if err = ValidateMembers(z.File); err != nil {
		return nil, err
	}
	var root string
	size := uint64(0)
	entries := map[string]*zip.File{}
	for _, f := range z.File {
		name := strings.TrimSuffix(f.Name, "/")
		rel := name
		if document {
			first, nestedName, nested := strings.Cut(name, "/")
			if root == "" {
				root = first
			}
			if root != first || !nested && !f.FileInfo().IsDir() {
				return nil, ErrArchive
			}
			rel = nestedName
		}
		if len(name) > 4096 || strings.Count(name, "/") > 64 {
			return nil, ErrArchive
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > 16<<20 || size+f.UncompressedSize64 > 48<<20 {
			return nil, ErrArchive
		}
		size += f.UncompressedSize64
		entries[rel] = f
	}
	if strings.HasPrefix(root, "data-20") && len(root) == len("data-20230321175442") {
		return nil, ErrArchive
	}
	files := make(map[string][]byte, len(entries))
	for name, f := range entries {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if entries[parent] != nil {
				return nil, ErrArchive
			}
		}
		input, err := f.Open()
		if err != nil {
			return nil, err
		}
		var output bytes.Buffer
		err = CopyContext(ctx, &output, io.LimitReader(input, int64(f.UncompressedSize64)+1))
		if err = errors.Join(err, input.Close()); err != nil {
			return nil, err
		}
		if uint64(output.Len()) != f.UncompressedSize64 {
			return nil, ErrArchive
		}
		files[name] = output.Bytes()
	}
	return files, ctx.Err()
}

// RemapDocumentIDs retains each block's creation time and sanitizes icon names.
func RemapDocumentIDs(tree *parse.Tree, ids map[string]string, newID func(string) string) error {
	if tree == nil || tree.Root == nil || !ast.IsNodeIDPattern(tree.Root.ID) {
		return ErrArchive
	}
	nodes := []*ast.Node{}
	seen := map[string]bool{}
	valid := true
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && n.ID != "" {
			if !ast.IsNodeIDPattern(n.ID) || seen[n.ID] || ids[n.ID] != "" {
				valid = false
				return ast.WalkStop
			}
			seen[n.ID] = true
			nodes = append(nodes, n)
		}
		return ast.WalkContinue
	})
	if !valid {
		return ErrArchive
	}
	for _, n := range nodes {
		id := newID(n.ID)
		if !ast.IsNodeIDPattern(id) {
			return ErrArchive
		}
		ids[n.ID] = id
		n.ID = id
		n.SetIALAttr("id", id)
		if icon := n.IALAttr("icon"); icon != "" {
			n.SetIALAttr("icon", textprefs.FilterUploadEmojiFileName(icon))
		}
	}
	tree.ID = tree.Root.ID
	return nil
}

func IDReplacer(ids map[string]string) *strings.Replacer {
	pairs := make([]string, 0, 2*len(ids))
	for _, old := range slices.Sorted(maps.Keys(ids)) {
		pairs = append(pairs, old, ids[old])
	}
	return strings.NewReplacer(pairs...)
}

// RemapDocumentReferences updates only the original three reference forms.
func RemapDocumentReferences(tree *parse.Tree, ids map[string]string, replace *strings.Replacer) {
	const blockLink = "siyuan://blocks/"
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if !enter {
			return ast.WalkContinue
		}
		switch {
		case n.Type == ast.NodeBlockRef || n.Type == ast.NodeTextMark && n.IsTextMarkType("block-ref"):
			if id := ids[n.TextMarkBlockRefID]; id != "" {
				n.TextMarkBlockRefID = id
			}
		case n.Type == ast.NodeTextMark && n.IsTextMarkType("a") && strings.HasPrefix(n.TextMarkAHref, blockLink):
			if id := ids[strings.TrimPrefix(n.TextMarkAHref, blockLink)]; id != "" {
				n.TextMarkAHref = blockLink + id
			}
		case n.Type == ast.NodeBlockQueryEmbedScript:
			n.Tokens = []byte(replace.Replace(string(n.Tokens)))
		}
		return ast.WalkContinue
	})
}
