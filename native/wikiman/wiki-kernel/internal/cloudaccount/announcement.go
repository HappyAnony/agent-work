// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudaccount

import (
	"context"
	"time"
	"wiki-kernel/internal/announcement"
)

func (s *Service) HasAnnouncements() bool { return s.announcements != nil }
func (s *Service) AnnouncementFeed(ctx context.Context) ([]announcement.Item, error) {
	if s.announcements == nil {
		return nil, ErrDependency
	}
	return s.announcements.List(ctx)
}
func (s *Service) RunAnnouncements(ctx context.Context, delay time.Duration) error {
	if s.announcements == nil {
		return ErrDependency
	}
	return s.announcements.Run(ctx, delay)
}
