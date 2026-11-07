package cloudaccount

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/announcement"
)

func TestAnnouncementDefaultRegionUsesSerializedAccountSnapshot(t *testing.T) {
	_, o := fixture(t)
	store, e := announcement.NewFileStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	o.Announcements = &announcement.Options{Store: store, CacheTTL: 6 * time.Hour, Fetch: func(ctx context.Context, _ int) (announcement.Response, error) {
		return announcement.Response{Status: 200, Body: map[string]any{"announcement": []any{}}}, ctx.Err()
	}, Notify: func(context.Context, announcement.Item) error { return nil }}
	s := openService(t, *o)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(region int) {
			defer wg.Done()
			for j := 0; j < 16; j++ {
				if region%2 == 0 {
					if _, e := s.Login(context.Background(), "fixture", "fixture", "", region); e != nil {
						t.Error(e)
					}
				} else {
					if _, e := s.AnnouncementFeed(context.Background()); e != nil && !errors.Is(e, announcement.ErrRegion) {
						t.Error(e)
					}
				}
			}
		}(i)
	}
	wg.Wait()
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e := s.AnnouncementFeed(context.Background()); !errors.Is(e, announcement.ErrClosed) {
		t.Fatal(e)
	}
}
