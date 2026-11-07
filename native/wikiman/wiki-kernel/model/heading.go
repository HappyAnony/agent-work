// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"context"
)

func (tx *Transaction) doFoldHeading(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoFoldHeading(operation))
}

func (tx *Transaction) doUnfoldHeading(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoUnfoldHeading(operation))
}

func Doc2Heading(srcID, targetID string, after bool) (string, string, error) {
	return Doc2HeadingContext(context.Background(), srcID, targetID, after)
}

func Heading2Doc(srcHeadingID, targetBoxID, targetPath, previousPath string) (srcRootBlockID, newTargetPath string, err error) {
	return Heading2DocContext(context.Background(), srcHeadingID, targetBoxID, targetPath, previousPath)
}
