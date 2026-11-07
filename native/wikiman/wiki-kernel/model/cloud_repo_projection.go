// SPDX-License-Identifier: AGPL-3.0-or-later
package model

// Kept identical to the original UI response, including the quota placeholders.
type cloudSpaceProjection struct {
	Sync                 *Sync   `json:"sync"`
	Backup               *Backup `json:"backup"`
	HSize                string  `json:"hSize"`
	HAssetSize           string  `json:"hAssetSize"`
	HTotalSize           string  `json:"hTotalSize"`
	HExchangeSize        string  `json:"hExchangeSize"`
	HTrafficUploadSize   string  `json:"hTrafficUploadSize"`
	HTrafficDownloadSize string  `json:"hTrafficDownloadSize"`
	HTrafficAPIGet       string  `json:"hTrafficAPIGet"`
	HTrafficAPIPut       string  `json:"hTrafficAPIPut"`
}
