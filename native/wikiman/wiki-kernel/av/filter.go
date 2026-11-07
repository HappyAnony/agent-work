// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Extracted from the original AV implementation; legacy UI aliases these types.
package av

import (
	"wiki-kernel/internal/avmodel"
)

type ViewFilter = avmodel.ViewFilter

type RelativeDateUnit = avmodel.RelativeDateUnit

const RelativeDateUnitDay = avmodel.RelativeDateUnitDay

const RelativeDateUnitWeek = avmodel.RelativeDateUnitWeek

const RelativeDateUnitMonth = avmodel.RelativeDateUnitMonth

const RelativeDateUnitYear = avmodel.RelativeDateUnitYear

type RelativeDateDirection = avmodel.RelativeDateDirection

const RelativeDateDirectionBefore = avmodel.RelativeDateDirectionBefore

const RelativeDateDirectionThis = avmodel.RelativeDateDirectionThis

const RelativeDateDirectionAfter = avmodel.RelativeDateDirectionAfter

type RelativeDate = avmodel.RelativeDate

type FilterOperator = avmodel.FilterOperator

const FilterOperatorIsEqual = avmodel.FilterOperatorIsEqual

const FilterOperatorIsNotEqual = avmodel.FilterOperatorIsNotEqual

const FilterOperatorIsGreater = avmodel.FilterOperatorIsGreater

const FilterOperatorIsGreaterOrEqual = avmodel.FilterOperatorIsGreaterOrEqual

const FilterOperatorIsLess = avmodel.FilterOperatorIsLess

const FilterOperatorIsLessOrEqual = avmodel.FilterOperatorIsLessOrEqual

const FilterOperatorContains = avmodel.FilterOperatorContains

const FilterOperatorDoesNotContain = avmodel.FilterOperatorDoesNotContain

const FilterOperatorIsEmpty = avmodel.FilterOperatorIsEmpty

const FilterOperatorIsNotEmpty = avmodel.FilterOperatorIsNotEmpty

const FilterOperatorStartsWith = avmodel.FilterOperatorStartsWith

const FilterOperatorEndsWith = avmodel.FilterOperatorEndsWith

const FilterOperatorIsBetween = avmodel.FilterOperatorIsBetween

const FilterOperatorIsTrue = avmodel.FilterOperatorIsTrue

const FilterOperatorIsFalse = avmodel.FilterOperatorIsFalse

type FilterQuantifier = avmodel.FilterQuantifier

const FilterQuantifierUndefined = avmodel.FilterQuantifierUndefined

const FilterQuantifierAny = avmodel.FilterQuantifierAny

const FilterQuantifierAll = avmodel.FilterQuantifierAll

const FilterQuantifierNone = avmodel.FilterQuantifierNone

func Filter(v Viewable, a *AttributeView, f map[string]Collection, c map[string]*AttributeView) {
	avmodel.Filter(v, bindEnvironment(a), f, c)
}
