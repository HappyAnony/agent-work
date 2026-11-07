// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Extracted from the original AV implementation; legacy UI aliases these types.
package av

import (
	"wiki-kernel/internal/avmodel"
)

type FieldCalc = avmodel.FieldCalc

type CalcOperator = avmodel.CalcOperator

const CalcOperatorNone = avmodel.CalcOperatorNone

const CalcOperatorUniqueValues = avmodel.CalcOperatorUniqueValues

const CalcOperatorCountAll = avmodel.CalcOperatorCountAll

const CalcOperatorCountValues = avmodel.CalcOperatorCountValues

const CalcOperatorCountUniqueValues = avmodel.CalcOperatorCountUniqueValues

const CalcOperatorCountEmpty = avmodel.CalcOperatorCountEmpty

const CalcOperatorCountNotEmpty = avmodel.CalcOperatorCountNotEmpty

const CalcOperatorPercentEmpty = avmodel.CalcOperatorPercentEmpty

const CalcOperatorPercentNotEmpty = avmodel.CalcOperatorPercentNotEmpty

const CalcOperatorPercentUniqueValues = avmodel.CalcOperatorPercentUniqueValues

const CalcOperatorSum = avmodel.CalcOperatorSum

const CalcOperatorAverage = avmodel.CalcOperatorAverage

const CalcOperatorMedian = avmodel.CalcOperatorMedian

const CalcOperatorMin = avmodel.CalcOperatorMin

const CalcOperatorMax = avmodel.CalcOperatorMax

const CalcOperatorRange = avmodel.CalcOperatorRange

const CalcOperatorEarliest = avmodel.CalcOperatorEarliest

const CalcOperatorLatest = avmodel.CalcOperatorLatest

const CalcOperatorChecked = avmodel.CalcOperatorChecked

const CalcOperatorUnchecked = avmodel.CalcOperatorUnchecked

const CalcOperatorPercentChecked = avmodel.CalcOperatorPercentChecked

const CalcOperatorPercentUnchecked = avmodel.CalcOperatorPercentUnchecked

func Calc(v Viewable, a *AttributeView) { avmodel.Calc(v, bindEnvironment(a)) }
