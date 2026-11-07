// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// ToolBox: unit tests for workspace_manager.go
// See docs/wiki-workspace-hotswitch/design.md §6.2 (T4 rollback / T6.4 drain).

package util

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// resetManager clears the in-memory state so each test starts from a
// known baseline. Must be called at the top of every test.
func resetManager() {
	wmPhase.Store(PhaseReady)
	wmMessage.Store("")
	wmSwitchFn = atomic.Value{}
}

func TestGetWorkspacePhase_DefaultsToReady(t *testing.T) {
	resetManager()
	if p := GetWorkspacePhase(); p != PhaseReady {
		t.Fatalf("expected PhaseReady, got %q", p)
	}
}

func TestSwitchWorkspace_NoPipelineRegistered(t *testing.T) {
	resetManager()
	err := SwitchWorkspace("/tmp/nowhere")
	if !errors.Is(err, ErrSwitchFuncNotRegistered) {
		t.Fatalf("expected ErrSwitchFuncNotRegistered, got %v", err)
	}
}

func TestSwitchWorkspace_SuccessTransitionsToReady(t *testing.T) {
	resetManager()
	WorkspaceDir = "/old"

	called := false
	RegisterWorkspaceSwitchFunc(func(oldDir, newDir string) error {
		called = true
		if oldDir != "/old" {
			t.Errorf("expected oldDir=/old, got %q", oldDir)
		}
		if newDir != "/new" {
			t.Errorf("expected newDir=/new, got %q", newDir)
		}
		// Pipeline is expected to update WorkspaceDir on success.
		WorkspaceDir = newDir
		return nil
	})

	err := SwitchWorkspace("/new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("pipeline was not invoked")
	}
	if p := GetWorkspacePhase(); p != PhaseReady {
		t.Fatalf("expected PhaseReady after success, got %q", p)
	}
	if WorkspaceDir != "/new" {
		t.Fatalf("expected WorkspaceDir=/new, got %q", WorkspaceDir)
	}
}

func TestSwitchWorkspace_FailureWithRollbackReturnsToReady(t *testing.T) {
	resetManager()
	WorkspaceDir = "/old"

	RegisterWorkspaceSwitchFunc(func(oldDir, newDir string) error {
		// Simulate pipeline failure WITH successful rollback:
		// WorkspaceDir stays at oldDir.
		return errors.New("init new workspace failed")
	})

	err := SwitchWorkspace("/new")
	if err == nil {
		t.Fatal("expected error")
	}
	// Rollback preserved /old, so phase should be Ready.
	if p := GetWorkspacePhase(); p != PhaseReady {
		t.Fatalf("expected PhaseReady after rollback, got %q", p)
	}
}

func TestSwitchWorkspace_FailureWithRollbackFailureGoesDegraded(t *testing.T) {
	resetManager()
	WorkspaceDir = "/old"

	RegisterWorkspaceSwitchFunc(func(oldDir, newDir string) error {
		// Simulate pipeline failure AND rollback failure:
		// WorkspaceDir is neither oldDir nor newDir.
		WorkspaceDir = "/broken"
		return errors.New("init failed and rollback failed")
	})

	err := SwitchWorkspace("/new")
	if err == nil {
		t.Fatal("expected error")
	}
	if p := GetWorkspacePhase(); p != PhaseDegraded {
		t.Fatalf("expected PhaseDegraded, got %q", p)
	}
}

func TestSwitchWorkspace_ConcurrentRequestFailsFast(t *testing.T) {
	resetManager()
	WorkspaceDir = "/old"

	block := make(chan struct{})
	RegisterWorkspaceSwitchFunc(func(oldDir, newDir string) error {
		<-block
		WorkspaceDir = newDir
		return nil
	})

	var wg sync.WaitGroup
	var firstErr, secondErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		firstErr = SwitchWorkspace("/new1")
	}()

	// Give the first goroutine time to enter the pipeline.
	time.Sleep(50 * time.Millisecond)

	// Second call should fail fast with ErrSwitchInProgress.
	secondErr = SwitchWorkspace("/new2")

	// Unblock the first call.
	close(block)
	wg.Wait()

	if firstErr != nil {
		t.Fatalf("first switch unexpectedly failed: %v", firstErr)
	}
	if !errors.Is(secondErr, ErrSwitchInProgress) {
		t.Fatalf("expected ErrSwitchInProgress for concurrent request, got %v", secondErr)
	}
	if WorkspaceDir != "/new1" {
		t.Fatalf("expected /new1 to win, got %q", WorkspaceDir)
	}
}

func TestGetWorkspaceStatus_ReflectsCurrentState(t *testing.T) {
	resetManager()
	WorkspaceDir = "/some/path"

	status := GetWorkspaceStatus()
	if status.CurrentDir != "/some/path" {
		t.Errorf("expected CurrentDir=/some/path, got %q", status.CurrentDir)
	}
	if status.Phase != PhaseReady {
		t.Errorf("expected PhaseReady, got %q", status.Phase)
	}
}
