package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
	"wiki-kernel/util"
)

func TestDAVStoreRejectsInvalidRootsAndPaths(t *testing.T) {
	for _, root := range []string{"", ".", "relative/root"} {
		if _, err := NewDAVStore(root); !errors.Is(err, errDAVRootInvalid) {
			t.Fatalf("NewDAVStore(%q) error = %v", root, err)
		}
	}

	root := t.TempDir()
	want := filepath.Join(root, "storage", "caldav", "principals", "main")
	if got, err := davStoragePath(root, "/caldav/principals/main"); err != nil || got != want {
		t.Fatalf("davStoragePath() = %q, %v; want %q", got, err, want)
	}
	for _, davPath := range []string{"", "/../escape", "//absolute"} {
		if _, err := davStoragePath(root, davPath); !errors.Is(err, errDAVPathInvalid) {
			t.Fatalf("davStoragePath(%q) error = %v", davPath, err)
		}
	}
}

func TestDAVStoresKeepOwnersAndSameIDsIsolated(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	storeA, err := NewDAVStore(rootA)
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := NewDAVStore(rootB)
	if err != nil {
		t.Fatal(err)
	}

	previousDataDir := util.DataDir
	thirdRoot := t.TempDir()
	util.DataDir = thirdRoot
	t.Cleanup(func() { util.DataDir = previousDataDir })

	ctx := context.Background()
	calA, calB := storeA.CalDavBackend(), storeB.CalDavBackend()
	cardA, cardB := storeA.CardDavBackend(), storeB.CardDavBackend()
	if _, err = calA.ListCalendars(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = calB.ListCalendars(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = cardA.ListAddressBooks(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = cardB.ListAddressBooks(ctx); err != nil {
		t.Fatal(err)
	}

	calendarPath := CalDavDefaultCalendarPath + "/same.ics"
	if _, err = calA.PutCalendarObject(ctx, calendarPath, testCalendar(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err = calB.PutCalendarObject(ctx, calendarPath, testCalendar(), nil); err != nil {
		t.Fatal(err)
	}

	cardPath := CardDavDefaultAddressBookPath + "/same.vcf"
	firstCard := vcard.Card{}
	firstCard.SetValue(vcard.FieldVersion, "4.0")
	firstCard.SetValue(vcard.FieldFormattedName, "Store A")
	secondCard := vcard.Card{}
	secondCard.SetValue(vcard.FieldVersion, "4.0")
	secondCard.SetValue(vcard.FieldFormattedName, "Store B")
	if _, err = cardA.PutAddressObject(ctx, cardPath, firstCard, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = cardB.PutAddressObject(ctx, cardPath, secondCard, nil); err != nil {
		t.Fatal(err)
	}

	for _, root := range []string{rootA, rootB} {
		for _, davPath := range []string{calendarPath, cardPath} {
			filePath, pathErr := davStoragePath(root, davPath)
			if pathErr != nil {
				t.Fatal(pathErr)
			}
			if _, statErr := os.Stat(filePath); statErr != nil {
				t.Fatalf("owned file %q: %v", filePath, statErr)
			}
		}
	}
	if data, readErr := os.ReadFile(filepath.Join(rootA, "storage", filepath.FromSlash(cardPath))); readErr != nil || string(data) == "" {
		t.Fatalf("store A card = %q, %v", data, readErr)
	} else if other, otherErr := os.ReadFile(filepath.Join(rootB, "storage", filepath.FromSlash(cardPath))); otherErr != nil || string(data) == string(other) {
		t.Fatalf("same-ID cards were not isolated: %v", otherErr)
	}
	if _, statErr := os.Stat(filepath.Join(thirdRoot, "storage")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("changed global data root was used: %v", statErr)
	}
}

func testCalendar() *ical.Calendar {
	calendar := ical.NewCalendar()
	calendar.Props.SetText(ical.PropVersion, "2.0")
	calendar.Props.SetText(ical.PropProductID, "-//AnonyPower//DAV ownership test//EN")
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, "same-event")
	event.Props.SetDateTime(ical.PropDateTimeStamp, time.Unix(0, 0).UTC())
	calendar.Children = append(calendar.Children, event.Component)
	return calendar
}

func TestDAVStoresDeepCopyDefaultsAndRejectUnboundBackends(t *testing.T) {
	storeA, err := NewDAVStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := NewDAVStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = storeA.CalDavBackend().ListCalendars(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = storeB.CalDavBackend().ListCalendars(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = storeA.CardDavBackend().ListAddressBooks(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = storeB.CardDavBackend().ListAddressBooks(ctx); err != nil {
		t.Fatal(err)
	}

	storeA.calendars.calendarsMetaData[0].Name = "changed"
	storeA.calendars.calendarsMetaData[0].SupportedComponentSet[0] = "changed"
	if got := storeB.calendars.calendarsMetaData[0]; got.Name == "changed" || got.SupportedComponentSet[0] == "changed" {
		t.Fatalf("calendar defaults shared across stores: %#v", got)
	}
	storeA.contacts.booksMetaData[0].Name = "changed"
	storeA.contacts.booksMetaData[0].SupportedAddressData[0].Version = "changed"
	if got := storeB.contacts.booksMetaData[0]; got.Name == "changed" || got.SupportedAddressData[0].Version == "changed" {
		t.Fatalf("address-book defaults shared across stores: %#v", got)
	}

	if _, err = (&CalDavBackend{}).ListCalendars(ctx); !errors.Is(err, errDAVOwnerUnbound) {
		t.Fatalf("unbound CalDAV backend error = %v", err)
	}
	if _, err = (&CardDavBackend{}).ListAddressBooks(ctx); !errors.Is(err, errDAVOwnerUnbound) {
		t.Fatalf("unbound CardDAV backend error = %v", err)
	}
}
