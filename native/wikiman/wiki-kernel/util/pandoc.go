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

package util

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/88250/gulu"
	"github.com/siyuan-note/logging"
)

var ErrPandocNotFound = errors.New("not found executable pandoc")

func ConvertPandoc(dir string, args ...string) (path string, err error) {
	if "" == PandocBinPath || ContainerStd != Container {
		err = ErrPandocNotFound
		return
	}

	pandoc := exec.Command(PandocBinPath, args...)
	gulu.CmdAttr(pandoc)
	path = filepath.Join("temp", "convert", "pandoc", dir)
	absPath := filepath.Join(WorkspaceDir, path)
	if err = os.MkdirAll(absPath, 0755); err != nil {
		logging.LogErrorf("mkdir [%s] failed: [%s]", absPath, err)
		return
	}
	pandoc.Dir = absPath
	output, err := pandoc.CombinedOutput()
	if err != nil {
		err = errors.Join(err, errors.New(string(output)))
		logging.LogErrorf("pandoc convert output failed: %s", err)
		return
	}
	path = "/" + filepath.ToSlash(path)
	return
}

func Pandoc(from, to, o, content string) (err error) {
	if "" == from || "" == to || "md" == to {
		if err = gulu.File.WriteFileSafer(o, []byte(content), 0644); err != nil {
			logging.LogErrorf("write export markdown file [%s] failed: %s", o, err)
		}
		return
	}

	if "" == PandocBinPath || ContainerStd != Container {
		err = ErrPandocNotFound
		return
	}

	dir := filepath.Join(WorkspaceDir, "temp", "convert", "pandoc", gulu.Rand.String(7))
	if err = os.MkdirAll(dir, 0755); err != nil {
		logging.LogErrorf("mkdir [%s] failed: [%s]", dir, err)
		return
	}
	tmpPath := filepath.Join(dir, gulu.Rand.String(7))
	if err = os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		logging.LogErrorf("write file failed: [%s]", err)
		return
	}

	args := []string{
		tmpPath,
		"--from", from,
		"--to", to,
		"--resource-path", filepath.Dir(o),
		"-s",
		"-o", o,
	}

	pandoc := exec.Command(PandocBinPath, args...)
	gulu.CmdAttr(pandoc)
	output, err := pandoc.CombinedOutput()
	if err != nil {
		logging.LogErrorf("pandoc convert output [%s], error [%s]", string(output), err)
		return
	}
	return
}

var (
	PandocBinPath string // Pandoc 可执行文件路径
)

type PandocInitConfig struct {
	Container  string
	TempDir    string
	ConfDir    string
	WorkingDir string
	Mode       string
}

type PandocInitResult struct {
	BinPath string
	Notify  bool
}

// CapturePandocInitConfig freezes workspace-sensitive paths before the
// process-level initializer leaves the boot goroutine.
func CapturePandocInitConfig() PandocInitConfig {
	return PandocInitConfig{
		Container:  Container,
		TempDir:    TempDir,
		ConfDir:    ConfDir,
		WorkingDir: WorkingDir,
		Mode:       Mode,
	}
}

func InitPandoc() {
	_, _ = InitPandocContext(context.Background(), CapturePandocInitConfig())
}

func InitPandocContext(ctx context.Context, config PandocInitConfig) (result PandocInitResult, err error) {
	if ctx == nil {
		return result, context.Canceled
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if ContainerStd != config.Container {
		return result, nil
	}

	pandocDir := filepath.Join(config.TempDir, "pandoc")

	if confPath := filepath.Join(config.ConfDir, "conf.json"); gulu.File.IsExist(confPath) {
		// Workspace built-in Pandoc is no longer initialized after customizing Pandoc path https://github.com/siyuan-note/siyuan/issues/8377
		if data, err := os.ReadFile(confPath); err == nil {
			conf := map[string]interface{}{}
			if err = gulu.JSON.UnmarshalJSON(data, &conf); err == nil {
				export, exportOK := conf["export"].(map[string]interface{})
				customPandocBinPath, pathOK := export["pandocBin"].(string)
				if exportOK && pathOK && !strings.HasPrefix(customPandocBinPath, pandocDir) {
					pandocVer, probeErr := getPandocVerContext(ctx, customPandocBinPath)
					if probeErr != nil {
						return result, probeErr
					}
					if "" != pandocVer {
						PandocBinPath = customPandocBinPath
						logging.LogInfof("custom pandoc [ver=%s, bin=%s]", pandocVer, PandocBinPath)
						result.BinPath = PandocBinPath
						return result, nil
					}
				}
			}
		}
	}

	result.Notify = true
	if err = ctx.Err(); err != nil {
		return result, err
	}

	if gulu.OS.IsWindows() {
		if "amd64" == runtime.GOARCH {
			PandocBinPath = filepath.Join(pandocDir, "bin", "pandoc.exe")
		}
	} else if gulu.OS.IsDarwin() {
		PandocBinPath = filepath.Join(pandocDir, "bin", "pandoc")
	} else if gulu.OS.IsLinux() {
		if "amd64" == runtime.GOARCH {
			PandocBinPath = filepath.Join(pandocDir, "bin", "pandoc")
		}
	}
	pandocVer, probeErr := getPandocVerContext(ctx, PandocBinPath)
	if probeErr != nil {
		return result, probeErr
	}
	if "" != pandocVer {
		logging.LogInfof("built-in pandoc [ver=%s, bin=%s]", pandocVer, PandocBinPath)
		result.BinPath = PandocBinPath
		return result, nil
	}

	pandocZip := filepath.Join(config.WorkingDir, "pandoc.zip")
	if "dev" == config.Mode || !gulu.File.IsExist(pandocZip) {
		if gulu.OS.IsWindows() {
			if "amd64" == runtime.GOARCH {
				pandocZip = filepath.Join(config.WorkingDir, "pandoc/pandoc-windows-amd64.zip")
			}
		} else if gulu.OS.IsDarwin() {
			if "amd64" == runtime.GOARCH {
				pandocZip = filepath.Join(config.WorkingDir, "pandoc/pandoc-darwin-amd64.zip")
			} else if "arm64" == runtime.GOARCH {
				pandocZip = filepath.Join(config.WorkingDir, "pandoc/pandoc-darwin-arm64.zip")
			}
		} else if gulu.OS.IsLinux() {
			if "amd64" == runtime.GOARCH {
				pandocZip = filepath.Join(config.WorkingDir, "pandoc/pandoc-linux-amd64.zip")
			} else if "arm64" == runtime.GOARCH {
				pandocZip = filepath.Join(config.WorkingDir, "pandoc/pandoc-linux-arm64.zip")
			}
		}
	}

	if !gulu.File.IsExist(pandocZip) {
		PandocBinPath = ""
		logging.LogErrorf("pandoc zip [%s] not found", pandocZip)
		return result, nil
	}

	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err := gulu.Zip.Unzip(pandocZip, pandocDir); err != nil {
		logging.LogErrorf("unzip pandoc failed: %s", err)
		return result, nil
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}

	if gulu.OS.IsDarwin() || gulu.OS.IsLinux() {
		chmod := exec.CommandContext(ctx, "chmod", "+x", PandocBinPath)
		chmod.WaitDelay = time.Second
		_, _ = chmod.CombinedOutput()
	}
	pandocVer, probeErr = getPandocVerContext(ctx, PandocBinPath)
	if probeErr != nil {
		return result, probeErr
	}
	logging.LogInfof("initialized built-in pandoc [ver=%s, bin=%s]", pandocVer, PandocBinPath)
	result.BinPath = PandocBinPath
	return result, nil
}

func getPandocVer(binPath string) (ret string) {
	ret, _ = getPandocVerContext(context.Background(), binPath)
	return
}

func getPandocVerContext(ctx context.Context, binPath string) (ret string, err error) {
	if "" == binPath {
		return "", nil
	}

	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probe, binPath, "--version")
	gulu.CmdAttr(cmd)
	cmd.WaitDelay = time.Second
	data, err := cmd.CombinedOutput()
	if parentErr := ctx.Err(); parentErr != nil {
		return "", parentErr
	}
	if err == nil && strings.HasPrefix(string(data), "pandoc") {
		parts := bytes.Split(data, []byte("\n"))
		if 0 < len(parts) {
			ret = strings.TrimPrefix(string(parts[0]), "pandoc")
			ret = strings.ReplaceAll(ret, ".exe", "")
			ret = strings.TrimSpace(ret)
		}
		return ret, nil
	}
	return "", nil
}

func IsValidPandocBin(path string) bool {
	valid, _ := ValidatePandocBin(context.Background(), path)
	return valid
}

// ValidatePandocBin probes only an explicitly supplied device-tool capability.
func ValidatePandocBin(ctx context.Context, path string) (bool, error) {
	if e := ctx.Err(); e != nil {
		return false, e
	}
	if path == "" {
		return false, nil
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probe, path, "--version")
	gulu.CmdAttr(cmd)
	cmd.WaitDelay = time.Second
	output := &pandocProbeOutput{}
	cmd.Stdout, cmd.Stderr = output, output
	e := cmd.Run()
	if parent := ctx.Err(); parent != nil {
		return false, parent
	}
	output.mu.Lock()
	defer output.mu.Unlock()
	return e == nil && strings.HasPrefix(string(output.data), "pandoc"), nil
}

type pandocProbeOutput struct {
	mu   sync.Mutex
	data []byte
}

func (b *pandocProbeOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := min(len(p), (64<<10)-len(b.data))
	if n > 0 {
		b.data = append(b.data, p[:n]...)
	}
	return len(p), nil
}
