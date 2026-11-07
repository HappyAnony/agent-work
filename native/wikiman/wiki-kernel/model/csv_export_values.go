// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"encoding/csv"
	"io"
	"strconv"
	"strings"
	"wiki-kernel/av"
)

// Original CSV display rules, shared by the request-owned controller. The
// rendered table is a detached candidate; date formatting never rewrites source.
func writeAttributeViewCSV(dst io.Writer, attrView *av.AttributeView, table *av.Table, guard func() error) (err error) {
	if err = guard(); err != nil {
		return err
	}
	n, err := io.WriteString(dst, "\xEF\xBB\xBF")
	if err != nil {
		return err
	}
	if n != 3 {
		return io.ErrShortWrite
	}
	writer := csv.NewWriter(dst)
	var header []string
	for _, col := range table.Columns {
		header = append(header, col.Name)
	}
	if err = writer.Write(header); err != nil {
		return err
	}
	rowNum := 1
	for _, row := range table.Rows {
		if err = guard(); err != nil {
			return err
		}
		var rowVal []string
		for _, cell := range row.Cells {
			var val string
			if nil != cell.Value {
				if av.KeyTypeDate == cell.Value.Type {
					if nil != cell.Value.Date {
						cell.Value.Date = av.NewFormattedValueDate(cell.Value.Date.Content, cell.Value.Date.Content2, av.DateFormatNone, cell.Value.Date.IsNotTime, cell.Value.Date.HasEndDate)
					}
				} else if av.KeyTypeCreated == cell.Value.Type {
					if nil != cell.Value.Created {
						key, _ := attrView.GetKey(cell.Value.KeyID)
						isNotTime := false
						if nil != key && nil != key.Created {
							isNotTime = !key.Created.IncludeTime
						}

						cell.Value.Created = av.NewFormattedValueCreated(cell.Value.Created.Content, 0, av.CreatedFormatNone, isNotTime)
					}
				} else if av.KeyTypeUpdated == cell.Value.Type {
					if nil != cell.Value.Updated {
						key, _ := attrView.GetKey(cell.Value.KeyID)
						isNotTime := false
						if nil != key && nil != key.Updated {
							isNotTime = !key.Updated.IncludeTime
						}

						cell.Value.Updated = av.NewFormattedValueUpdated(cell.Value.Updated.Content, 0, av.UpdatedFormatNone, isNotTime)
					}
				} else if av.KeyTypeMAsset == cell.Value.Type {
					if nil != cell.Value.MAsset {
						buf := &bytes.Buffer{}
						for _, a := range cell.Value.MAsset {
							if av.AssetTypeImage == a.Type {
								buf.WriteString("![")
								buf.WriteString(a.Name)
								buf.WriteString("](")
								buf.WriteString(a.Content)
								buf.WriteString(") ")
							} else if av.AssetTypeFile == a.Type {
								buf.WriteString("[")
								buf.WriteString(a.Name)
								buf.WriteString("](")
								buf.WriteString(a.Content)
								buf.WriteString(") ")
							} else {
								buf.WriteString(a.Content)
								buf.WriteString(" ")
							}
						}
						val = strings.TrimSpace(buf.String())
					}
				} else if av.KeyTypeLineNumber == cell.Value.Type {
					val = strconv.Itoa(rowNum)
				}

				if "" == val {
					val = cell.Value.String(true)
				}
			}

			rowVal = append(rowVal, val)
		}
		if err = writer.Write(rowVal); err != nil {
			return err
		}
		rowNum++
	}
	writer.Flush()
	if err = writer.Error(); err != nil {
		return err
	}
	return guard()
}
