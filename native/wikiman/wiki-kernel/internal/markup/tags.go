// SPDX-License-Identifier: AGPL-3.0-or-later
// Original setNodeAttrs0 tag-list and attribute-value normalization.
package markup

import (
	"github.com/88250/gulu"
	"strings"
)

func NormalizeTagList(tag string) string {
	var tags []string
	for _, t := range strings.Split(tag, ",") {
		t = strings.TrimSpace(gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(t)))
		if t != "" {
			tags = append(tags, t)
		}
	}
	tags = gulu.Str.RemoveDuplicatedElem(tags)
	if len(tags) > 0 {
		return strings.Join(tags, ",")
	}
	return tag
}

func NormalizeTagValue(tag string) string {
	v := NormalizeTagList(tag)
	v = strings.ReplaceAll(v, "\u00a0", " ")
	v = gulu.Str.RemoveZeroWidthNoBreakSpace(v)
	v = gulu.Str.RemoveZeroWidthSpace(v)
	v = gulu.Str.RemovePUA(v)
	return strings.TrimSuffix(strings.TrimSpace(v), ",")
}
