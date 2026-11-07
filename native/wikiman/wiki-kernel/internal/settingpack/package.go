// SPDX-License-Identifier: AGPL-3.0-or-later
package settingpack

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/88250/gulu"
	"wiki-kernel/conf"
)

var ErrPackage = errors.New("invalid_setting_package")

// Projection is the original portable configuration export policy. It works on
// a detached owner snapshot; device paths/identity and private authority retain
// the original exclusions. Provider packages use their original separate form.
func Projection(ctx context.Context, kind string, data conf.AppData) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	switch kind {
	case "syncWebDAV":
		if data.Sync == nil {
			return nil, ErrPackage
		}
		return gulu.JSON.MarshalJSON(data.Sync.WebDAV)
	case "syncS3":
		if data.Sync == nil {
			return nil, ErrPackage
		}
		return gulu.JSON.MarshalJSON(data.Sync.S3)
	case "conf":
		clone, e := data.Clone()
		if e != nil {
			return nil, e
		}
		if clone.Appearance != nil {
			clone.Appearance.DarkThemes = nil
			clone.Appearance.LightThemes = nil
			clone.Appearance.Icons = nil
		}
		if clone.Editor != nil {
			clone.Editor.Emoji = []string{}
		}
		if clone.Export != nil {
			clone.Export.PandocBin = ""
		}
		clone.UserData = ""
		clone.Account = nil
		clone.AccessAuthCode = ""
		if clone.System != nil {
			clone.System.ID = ""
			clone.System.Name = ""
			clone.System.OSPlatform = ""
			clone.System.Container = ""
			clone.System.IsMicrosoftStore = false
			clone.System.IsInsider = false
			clone.System.MicrosoftDefenderExcluded = false
		}
		clone.Sync = nil
		clone.Stat = nil
		clone.Api = nil
		clone.Repo = nil
		clone.Publish = nil
		clone.CloudRegion = 0
		clone.DataIndexState = 0
		return gulu.JSON.MarshalIndentJSON(clone, "", "  ")
	default:
		return nil, ErrPackage
	}
}

// Preserve the existing on-disk provider format. The legacy device supplies the
// selected key; shared/native code never reads a process key or invents one.
func EncodeProvider(ctx context.Context, raw, key []byte) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if len(raw) > conf.MaxImportJSON/4-aes.BlockSize {
		return nil, ErrPackage
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	data := []byte(hex.EncodeToString(raw))
	padding := block.BlockSize() - len(data)%block.BlockSize()
	data = append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
	output := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, []byte("RandomInitVector")).CryptBlocks(output, data)
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return []byte(hex.EncodeToString(output)), nil
}
func DecodeProvider(ctx context.Context, raw, key []byte) ([]byte, error) {
	plaintext, e := DecodeCipher(ctx, raw, key)
	if e != nil {
		return nil, e
	}
	decoded, e := hex.DecodeString(string(plaintext))
	if e != nil || !json.Valid(decoded) {
		return nil, ErrPackage
	}
	return decoded, ctx.Err()
}

// DecodeCipher validates the shared legacy envelope before decoding the
// payload format selected by its caller.
func DecodeCipher(ctx context.Context, raw, key []byte) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if len(raw) > conf.MaxImportJSON {
		return nil, ErrPackage
	}
	data, e := hex.DecodeString(string(raw))
	if e != nil {
		return nil, ErrPackage
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	if len(data) == 0 || len(data)%block.BlockSize() != 0 {
		return nil, ErrPackage
	}
	plaintext := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, []byte("RandomInitVector")).CryptBlocks(plaintext, data)
	padding := int(plaintext[len(plaintext)-1])
	if padding < 1 || padding > block.BlockSize() || padding > len(plaintext) {
		return nil, ErrPackage
	}
	for _, b := range plaintext[len(plaintext)-padding:] {
		if int(b) != padding {
			return nil, ErrPackage
		}
	}
	return plaintext[:len(plaintext)-padding], ctx.Err()
}
