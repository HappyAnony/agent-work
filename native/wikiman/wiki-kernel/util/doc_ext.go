package util

import "strings"

const (
	DocExt    = ".docj"
	DocZipExt = ".docj.zip"
)

func HasDocExt(s string) bool {
	return strings.HasSuffix(s, DocExt)
}

func StripDocExt(s string) string {
	return strings.TrimSuffix(s, DocExt)
}

func AppendDocExt(id string) string {
	return id + DocExt
}

func HasDocZipExt(s string) bool {
	return strings.HasSuffix(s, DocZipExt)
}

func StripDocZipExt(s string) string {
	return strings.TrimSuffix(s, DocZipExt)
}
