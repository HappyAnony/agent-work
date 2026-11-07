// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Extracted from the original AV implementation; legacy UI aliases these types.
package av

import (
	"time"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/util"
)

type Value = avmodel.Value

const CheckboxCheckedStr = avmodel.CheckboxCheckedStr

type ValueBlock = avmodel.ValueBlock

type ValueText = avmodel.ValueText

type ValueNumber = avmodel.ValueNumber

type NumberFormat = avmodel.NumberFormat

const NumberFormatNone = avmodel.NumberFormatNone

const NumberFormatCommas = avmodel.NumberFormatCommas

const NumberFormatPercent = avmodel.NumberFormatPercent

const NumberFormatUSD = avmodel.NumberFormatUSD

const NumberFormatCNY = avmodel.NumberFormatCNY

const NumberFormatEUR = avmodel.NumberFormatEUR

const NumberFormatGBP = avmodel.NumberFormatGBP

const NumberFormatJPY = avmodel.NumberFormatJPY

const NumberFormatRUB = avmodel.NumberFormatRUB

const NumberFormatINR = avmodel.NumberFormatINR

const NumberFormatKRW = avmodel.NumberFormatKRW

const NumberFormatCAD = avmodel.NumberFormatCAD

const NumberFormatCHF = avmodel.NumberFormatCHF

const NumberFormatTHB = avmodel.NumberFormatTHB

const NumberFormatAUD = avmodel.NumberFormatAUD

const NumberFormatHKD = avmodel.NumberFormatHKD

const NumberFormatTWD = avmodel.NumberFormatTWD

const NumberFormatMOP = avmodel.NumberFormatMOP

const NumberFormatSGD = avmodel.NumberFormatSGD

const NumberFormatNZD = avmodel.NumberFormatNZD

var NewFormattedValueNumber = avmodel.NewFormattedValueNumber

type ValueDate = avmodel.ValueDate

type DateFormat = avmodel.DateFormat

const DateFormatNone = avmodel.DateFormatNone

const DateFormatDuration = avmodel.DateFormatDuration

func NewFormattedValueDate(content, content2 int64, format DateFormat, isNotTime, hasEndDate bool) (ret *ValueDate) {
	return avmodel.NewFormattedValueDate(content, content2, format, isNotTime, hasEndDate, func(a, b time.Time) string { return util.HumanizeRelTime(a, b, util.Lang) })
}

var RoundUp = avmodel.RoundUp

var RoundDown = avmodel.RoundDown

var Round = avmodel.Round

type ValueSelect = avmodel.ValueSelect

var MSelectRemoveOption = avmodel.MSelectRemoveOption

var MSelectExistOption = avmodel.MSelectExistOption

type ValueURL = avmodel.ValueURL

type ValueEmail = avmodel.ValueEmail

type ValuePhone = avmodel.ValuePhone

type AssetType = avmodel.AssetType

const AssetTypeFile = avmodel.AssetTypeFile

const AssetTypeImage = avmodel.AssetTypeImage

type ValueAsset = avmodel.ValueAsset

type ValueTemplate = avmodel.ValueTemplate

type ValueCreated = avmodel.ValueCreated

type CreatedFormat = avmodel.CreatedFormat

const CreatedFormatNone = avmodel.CreatedFormatNone

const CreatedFormatDuration = avmodel.CreatedFormatDuration

func NewFormattedValueCreated(content, content2 int64, format CreatedFormat, isNotTime bool) (ret *ValueCreated) {
	return avmodel.NewFormattedValueCreated(content, content2, format, isNotTime, func(a, b time.Time) string { return util.HumanizeRelTime(a, b, util.Lang) })
}

type ValueUpdated = avmodel.ValueUpdated

type UpdatedFormat = avmodel.UpdatedFormat

const UpdatedFormatNone = avmodel.UpdatedFormatNone

const UpdatedFormatDuration = avmodel.UpdatedFormatDuration

func NewFormattedValueUpdated(content, content2 int64, format UpdatedFormat, isNotTime bool) (ret *ValueUpdated) {
	return avmodel.NewFormattedValueUpdated(content, content2, format, isNotTime, func(a, b time.Time) string { return util.HumanizeRelTime(a, b, util.Lang) })
}

type ValueCheckbox = avmodel.ValueCheckbox

type ValueRelation = avmodel.ValueRelation

type ValueRollup = avmodel.ValueRollup

var GetAttributeViewDefaultValue = avmodel.GetAttributeViewDefaultValue
