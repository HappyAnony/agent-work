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

//go:build windows

package model

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"wiki-kernel/conf"

	"golang.org/x/sys/windows"

	"github.com/88250/gulu"
	"wiki-kernel/util"
)

var microsoftDefenderLock = sync.Mutex{}

func AddMicrosoftDefenderExclusion() error {
	return AddMicrosoftDefenderExclusionContext(context.Background())
}
func AddMicrosoftDefenderExclusionContext(ctx context.Context) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	ui, e := captureLegacyDelayedUIOwner(ctx)
	if e != nil {
		return e
	}
	working, workspace, mode := util.WorkingDir, base.workspace, util.Mode
	return base.run(ctx, func(c, _ context.Context) error {
		microsoftDefenderLock.Lock()
		defer microsoftDefenderLock.Unlock()
		if e := base.guard(c); e != nil {
			return e
		}
		if !isUsingMicrosoftDefenderContext(c) {
			return c.Err()
		}
		elevator := getElevatorBinAt(working, mode)
		if !gulu.File.IsExist(elevator) {
			return fmt.Errorf("not found elevator [%s]", elevator)
		}
		install := filepath.Dir(working)
		ps := []string{"powershell", "-Command", "Add-MpPreference", "-ExclusionPath", install, ",", workspace}
		verbPtr, e := syscall.UTF16PtrFromString("runas")
		if e != nil {
			return e
		}
		exePtr, e := syscall.UTF16PtrFromString(elevator)
		if e != nil {
			return e
		}
		cwdPtr, e := syscall.UTF16PtrFromString(working)
		if e != nil {
			return e
		}
		argsPtr, e := syscall.UTF16PtrFromString(strings.Join(ps, " "))
		if e != nil {
			return e
		}
		if working != util.WorkingDir {
			return conf.ErrBusinessUnbound
		}
		// ShellExecute hands this explicit device request to Windows/UAC. The OS
		// owns the elevation prompt; no application background worker is detached.
		if e = windows.ShellExecute(0, verbPtr, exePtr, argsPtr, cwdPtr, 1); e != nil {
			return e
		}
		if e = base.guard(c); e != nil {
			return e
		}
		if _, e = SetDeviceConfigurationContext(c, "defenderExcluded", []byte(`{}`)); e != nil {
			return e
		}
		if !ui.publish(c, func() { util.PushMsg(ui.language(102, false), 5000) }) {
			return conf.ErrBusinessUnbound
		}
		return nil
	})
}
func AutoCheckMicrosoftDefenderJob() {
	logOwnedTaskError("Defender reminder", AutoCheckMicrosoftDefenderJobContext(context.Background()))
}
func AutoCheckMicrosoftDefenderJobContext(ctx context.Context) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	ui, e := captureLegacyDelayedUIOwner(ctx)
	if e != nil {
		return e
	}
	working, mode := util.WorkingDir, util.Mode
	return base.run(ctx, func(c, _ context.Context) error {
		timer := time.NewTimer(7 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-c.Done():
			return c.Err()
		}
		microsoftDefenderLock.Lock()
		defer microsoftDefenderLock.Unlock()
		if e := base.guard(c); e != nil {
			return e
		}
		excluded := false
		if e := base.owner.WithSettingSnapshot(c, func(v conf.AppData) error {
			if v.System == nil {
				return conf.ErrBusinessUnbound
			}
			excluded = v.System.MicrosoftDefenderExcluded
			return nil
		}); e != nil {
			return e
		}
		if excluded || !gulu.File.IsExist(getElevatorBinAt(working, mode)) {
			return nil
		}
		if !isUsingMicrosoftDefenderContext(c) {
			return c.Err()
		}
		if !ui.publish(c, func() { util.PushMsg(ui.language(252, false), 0) }) {
			return conf.ErrBusinessUnbound
		}
		return nil
	})
}

func isUsingMicrosoftDefender() bool { return isUsingMicrosoftDefenderContext(context.Background()) }
func isUsingMicrosoftDefenderContext(ctx context.Context) bool {
	if !gulu.OS.IsWindows() {
		return false
	}

	cmd := exec.CommandContext(ctx, "powershell", "-Command", "Get-MpPreference")
	cmd.WaitDelay = time.Second
	gulu.CmdAttr(cmd)
	return cmd.Run() == nil
}

func getElevatorBin() string { return getElevatorBinAt(util.WorkingDir, util.Mode) }
func getElevatorBinAt(working, mode string) string {
	elevator := filepath.Join(working, "kernel", "elevator.exe")
	if "dev" == mode || !gulu.File.IsExist(elevator) {
		elevator = filepath.Join(working, "elevator", "elevator-"+runtime.GOARCH+".exe")
	}
	return elevator
}
