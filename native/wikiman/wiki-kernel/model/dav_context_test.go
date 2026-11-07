package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/caldav"
	"github.com/emersion/go-webdav/carddav"
)

func TestDAVTasksBoundConcurrencyAndPropagateCancellation(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	var completed atomic.Int32
	release := make(chan struct{})
	tasks := make([]func(context.Context) error, 32)
	for i := range tasks {
		tasks[i] = func(ctx context.Context) error {
			current := active.Add(1)
			defer active.Add(-1)
			for {
				previous := maximum.Load()
				if current <= previous || maximum.CompareAndSwap(previous, current) {
					break
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-release:
				completed.Add(1)
				return nil
			}
		}
	}
	done := make(chan error, 1)
	go func() { done <- runDAVTasks(context.Background(), tasks) }()
	deadline := time.After(3 * time.Second)
	for maximum.Load() < davWorkerLimit {
		select {
		case err := <-done:
			t.Fatalf("tasks returned early: %v", err)
		case <-deadline:
			t.Fatal("bounded workers did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := maximum.Load(); got != davWorkerLimit {
		t.Fatalf("maximum workers = %d", got)
	}
	if got := completed.Load(); got != int32(len(tasks)) {
		t.Fatalf("completed tasks = %d", got)
	}

	sentinel := errors.New("dav worker failed")
	observedCancel := make(chan struct{})
	siblingReady := make(chan struct{})
	if err := runDAVTasks(context.Background(), []func(context.Context) error{
		func(context.Context) error {
			<-siblingReady
			return sentinel
		},
		func(ctx context.Context) error {
			close(siblingReady)
			<-ctx.Done()
			close(observedCancel)
			return ctx.Err()
		},
	}); !errors.Is(err, sentinel) {
		t.Fatalf("worker error = %v", err)
	}
	select {
	case <-observedCancel:
	default:
		t.Fatal("sibling did not observe cancellation")
	}
}

func TestDAVLoadContextRejectsCancellationAndChildErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&CalDavBackend{}).CurrentUserPrincipal(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("CalDAV backend cancellation = %v", err)
	}
	if _, err := (&CardDavBackend{}).CurrentUserPrincipal(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("CardDAV backend cancellation = %v", err)
	}
	if err := (&Calendars{}).LoadContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("calendars cancellation = %v", err)
	}
	if err := (&Contacts{}).LoadContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("contacts cancellation = %v", err)
	}

	calendarDir := t.TempDir()
	if err := os.Symlink(filepath.Join(calendarDir, "missing.ics"), filepath.Join(calendarDir, "broken.ics")); err != nil {
		t.Fatal(err)
	}
	calendar := &Calendar{DirectoryPath: calendarDir, MetaData: &caldav.Calendar{Path: CalDavDefaultCalendarPath}}
	if err := calendar.loadContext(context.Background()); err == nil {
		t.Fatal("calendar child error was swallowed")
	}

	addressDir := t.TempDir()
	if err := os.Symlink(filepath.Join(addressDir, "missing.vcf"), filepath.Join(addressDir, "broken.vcf")); err != nil {
		t.Fatal(err)
	}
	book := &AddressBook{DirectoryPath: addressDir}
	if err := book.loadContext(context.Background()); err == nil {
		t.Fatal("address child error was swallowed")
	}
}

func TestDAVSaveContextPropagatesChildErrorsAndKeepsDirtyState(t *testing.T) {
	blockedParent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blockedParent, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	calendar := &Calendar{Changed: true}
	calendar.Objects.Store("broken.ics", &CalendarObject{
		Changed:  true,
		FilePath: filepath.Join(blockedParent, "broken.ics"),
		Data:     &caldav.CalendarObject{Data: ical.NewCalendar()},
	})
	if err := calendar.saveContext(context.Background(), true); err == nil {
		t.Fatal("calendar save child error was swallowed")
	}
	if !calendar.Changed {
		t.Fatal("calendar dirty state was cleared after failure")
	}

	book := &AddressBook{Changed: true}
	book.Addresses.Store("broken.vcf", &AddressObject{
		Changed:  true,
		FilePath: filepath.Join(blockedParent, "broken.vcf"),
		Data:     &carddav.AddressObject{Card: vcard.Card{}},
	})
	if err := book.saveContext(context.Background(), true); err == nil {
		t.Fatal("address save child error was swallowed")
	}
	if !book.Changed {
		t.Fatal("address book dirty state was cleared after failure")
	}
}

func TestCardDAVMultiCardSplitKeepsDistinctIdentity(t *testing.T) {
	dir := t.TempDir()
	raw := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:One\r\nN:One;;;;\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Two\r\nN:Two;;;;\r\nEND:VCARD\r\n"
	original := filepath.Join(dir, "combined.vcf")
	if err := os.WriteFile(original, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	book := &AddressBook{
		DirectoryPath: dir,
		MetaData:      &carddav.AddressBook{Path: CardDavDefaultAddressBookPath},
	}
	if err := book.loadContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(original); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("combined source still exists: %v", err)
	}
	identities := map[string]bool{}
	book.Addresses.Range(func(key, value any) bool {
		address := value.(*AddressObject)
		id := key.(string)
		if id != filepath.Base(address.FilePath) {
			t.Fatalf("identity %q does not match %q", id, address.FilePath)
		}
		identities[id] = true
		return true
	})
	if len(identities) != 2 {
		t.Fatalf("split identities = %v", identities)
	}
}
