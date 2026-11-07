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

// ToolBox: Workspace hot-switch manager.
// See docs/wiki-workspace-hotswitch/design.md for the full design.
//
// This package introduces a process-level state machine that tracks workspace
// switching phases, serializes concurrent switch requests, and exposes a
// thin wrapper around the switch pipeline. The actual pipeline is implemented
// by a callback registered from higher-level packages (model/api) to avoid
// util -> model -> util import cycles.

package util

import (
	"errors"
	"sync"
	"sync/atomic"
)

// WorkspacePhase represents the current state of the workspace lifecycle.
type WorkspacePhase string

const (
	// PhaseReady means the kernel is serving a valid workspace normally.
	PhaseReady WorkspacePhase = "ready"
	// PhaseSwitching means a workspace switch is in progress; non-diagnostic
	// requests should be rejected with 503 by the concurrency middleware.
	PhaseSwitching WorkspacePhase = "switching"
	// PhaseDegraded means the kernel failed to initialize the new workspace
	// and also failed to roll back; only diagnostic APIs work. Recovery path
	// is to invoke switchWorkspaceDir again with a valid path.
	PhaseDegraded WorkspacePhase = "degraded"
)

// WorkspaceStatus is the snapshot returned by getWorkspaceStatus API.
type WorkspaceStatus struct {
	CurrentDir string         `json:"currentDir"`
	Phase      WorkspacePhase `json:"phase"`
	Message    string         `json:"message"`
}

// SwitchFunc is the actual switching pipeline, provided by higher-level
// packages (model) via RegisterSwitchFunc. It is expected to:
//  1. Tear down the old workspace resources (Step 2 in the design doc)
//  2. Initialize the new workspace resources (Step 3)
//  3. Return an error if the switch failed after attempted rollback
//
// The WorkspaceManager takes care of:
//  - Serialising concurrent requests (fail-fast via TryLock)
//  - Phase transitions (Ready -> Switching -> Ready/Degraded)
//  - The IsExiting flag toggling around the pipeline
type SwitchFunc func(oldDir, newDir string) error

var (
	wmMu      sync.RWMutex
	wmSwitch  sync.Mutex // serialize Switch calls (TryLock, fail-fast)
	wmPhase   atomic.Value // WorkspacePhase
	wmMessage atomic.Value // string
	wmSwitchFn atomic.Value // SwitchFunc
)

func init() {
	wmPhase.Store(PhaseReady)
	wmMessage.Store("")
}

// GetWorkspacePhase returns the current phase. Lock-free via atomic.Value.
func GetWorkspacePhase() WorkspacePhase {
	v := wmPhase.Load()
	if v == nil {
		return PhaseReady
	}
	return v.(WorkspacePhase)
}

// GetWorkspaceMessage returns the current phase description.
func GetWorkspaceMessage() string {
	v := wmMessage.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

// setWorkspacePhase stores a new phase and optional message together.
func setWorkspacePhase(phase WorkspacePhase, msg string) {
	wmPhase.Store(phase)
	wmMessage.Store(msg)
}

// GetWorkspaceStatus returns a snapshot of the current state.
func GetWorkspaceStatus() WorkspaceStatus {
	return WorkspaceStatus{
		CurrentDir: WorkspaceDir,
		Phase:      GetWorkspacePhase(),
		Message:    GetWorkspaceMessage(),
	}
}

// RegisterWorkspaceSwitchFunc registers the actual switch pipeline. This is
// called once from main during startup (after model import graph is loaded)
// to break the util -> model cycle.
func RegisterWorkspaceSwitchFunc(fn SwitchFunc) {
	wmSwitchFn.Store(fn)
}

// ErrSwitchInProgress is returned when a switch request arrives while
// another switch is already running. Callers should surface 409 Conflict.
var ErrSwitchInProgress = errors.New("another workspace switch is in progress")

// ErrSwitchFuncNotRegistered indicates the model layer has not yet
// registered a switch pipeline.
var ErrSwitchFuncNotRegistered = errors.New("workspace switch pipeline not registered")

// SwitchWorkspace drives a workspace switch through the phase state machine.
//
// Serialization: uses TryLock on wmSwitch to fail fast if a switch is already
// running. This matches design §3.2.13 (prefer 409 Conflict over blocking).
//
// Phase transitions:
//   Ready -> Switching -> Ready (success)
//   Ready -> Switching -> Degraded (failure + rollback failure)
//   Degraded -> Switching -> Ready (recovery path)
//
// The caller must hold the IsExiting invariant: this function sets
// IsExiting=true before invoking the pipeline so in-flight cron/sync
// short-circuit, and sets it back to false before Step 3 (inside the
// pipeline). The pipeline is responsible for toggling IsExiting back
// exactly once before calling any init functions.
func SwitchWorkspace(newPath string) error {
	if !wmSwitch.TryLock() {
		return ErrSwitchInProgress
	}
	defer wmSwitch.Unlock()

	fnVal := wmSwitchFn.Load()
	if fnVal == nil {
		return ErrSwitchFuncNotRegistered
	}
	fn := fnVal.(SwitchFunc)

	oldDir := WorkspaceDir

	wmMu.Lock()
	setWorkspacePhase(PhaseSwitching, "Switching workspace to "+newPath)
	wmMu.Unlock()

	err := fn(oldDir, newPath)

	wmMu.Lock()
	if err != nil {
		// Pipeline is expected to have attempted rollback. If the current
		// WorkspaceDir is still oldDir, the rollback succeeded and we can
		// resume Ready. Otherwise we are stuck in Degraded until the next
		// successful switch.
		if WorkspaceDir == oldDir {
			setWorkspacePhase(PhaseReady, "Switch failed, rolled back to "+oldDir+": "+err.Error())
		} else {
			setWorkspacePhase(PhaseDegraded, "Switch and rollback both failed: "+err.Error())
		}
	} else {
		setWorkspacePhase(PhaseReady, "Switched to "+newPath)
	}
	wmMu.Unlock()

	return err
}
