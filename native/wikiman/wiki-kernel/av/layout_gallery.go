// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Extracted from the original AV implementation; legacy UI aliases these types.
package av

import (
	"wiki-kernel/internal/avmodel"
)

type LayoutGallery = avmodel.LayoutGallery

var NewLayoutGallery = avmodel.NewLayoutGallery

type CardAspectRatio = avmodel.CardAspectRatio

const CardAspectRatio16_9 = avmodel.CardAspectRatio16_9

const CardAspectRatio9_16 = avmodel.CardAspectRatio9_16

const CardAspectRatio4_3 = avmodel.CardAspectRatio4_3

const CardAspectRatio3_4 = avmodel.CardAspectRatio3_4

const CardAspectRatio3_2 = avmodel.CardAspectRatio3_2

const CardAspectRatio2_3 = avmodel.CardAspectRatio2_3

const CardAspectRatio1_1 = avmodel.CardAspectRatio1_1

type CardSize = avmodel.CardSize

const CardSizeSmall = avmodel.CardSizeSmall

const CardSizeMedium = avmodel.CardSizeMedium

const CardSizeLarge = avmodel.CardSizeLarge

type CoverFrom = avmodel.CoverFrom

const CoverFromNone = avmodel.CoverFromNone

const CoverFromContentImage = avmodel.CoverFromContentImage

const CoverFromAssetField = avmodel.CoverFromAssetField

const CoverFromContentBlock = avmodel.CoverFromContentBlock

type ViewGalleryCardField = avmodel.ViewGalleryCardField

type Gallery = avmodel.Gallery

type GalleryCard = avmodel.GalleryCard

type GalleryField = avmodel.GalleryField

type GalleryFieldValue = avmodel.GalleryFieldValue
