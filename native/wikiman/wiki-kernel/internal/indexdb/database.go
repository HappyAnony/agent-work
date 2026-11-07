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

package indexdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-sqlite3"
)

var ErrClosed = errors.New("native_indexes_closed")
var registerOnce sync.Once

// The only process-wide state is an immutable driver definition, installed by
// explicit Open rather than importing the legacy SQL/model runtime.
func register() {
	registerOnce.Do(func() {
		sql.Register("sqlite3_extended", &sqlite3.SQLiteDriver{ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.RegisterFunc("regexp", func(re, text string) (bool, error) {
				return regexp.MatchString(strings.ReplaceAll(re, "\\\\", "\\"), text)
			}, true)
		}})
	})
}

func OpenFile(ctx context.Context, location string, pool int) (*sql.DB, error) {
	if location == "" {
		return nil, errors.New("index_path_required")
	}
	// Preserve the original explicit relative-workspace CLI input without a
	// process-wide chdir. Context services use registered roots or memory only.
	absolute, err := filepath.Abs(location)
	if err != nil {
		return nil, err
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String()
	return open(ctx, uri+"?", pool)
}

// Original derived-index connection settings. Source durability belongs to
// sourcetx; none of these disposable databases is the authoritative source.
func open(ctx context.Context, uri string, pool int) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if pool < 1 {
		return nil, errors.New("invalid_index_connection_budget")
	}
	register()
	db, err := sql.Open("sqlite3_extended", uri+"_journal_mode=WAL&_synchronous=OFF&_mmap_size=2684354560&_secure_delete=OFF&_cache_size=-20480&_page_size=32768&_busy_timeout=7000&_ignore_check_constraints=ON&_temp_store=MEMORY&_case_sensitive_like=OFF")
	if err != nil {
		return nil, err
	}
	db.SetMaxIdleConns(pool)
	db.SetMaxOpenConns(pool)
	db.SetConnMaxLifetime(365 * 24 * time.Hour)
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

type Bundle struct {
	mu       sync.Mutex
	dbs      map[Kind]*sql.DB
	keepers  map[Kind]*sql.Conn
	closed   bool
	closeErr error
}

// OpenMemory owns all four original index schemas, separately from the byte-
// span projection. It creates no source, default config or runtime files. Each
// DB has a keeper surviving cancelled transactions and one work connection.
func OpenMemory(ctx context.Context) (*Bundle, error) { return openMemory(ctx, Reset) }

func openMemory(ctx context.Context, reset func(context.Context, *sql.DB, Kind, string) error) (*Bundle, error) {
	b := &Bundle{dbs: map[Kind]*sql.DB{}, keepers: map[Kind]*sql.Conn{}}
	for _, kind := range []Kind{Main, History, Assets, BlockTree} {
		db, err := open(ctx, "file:wiki-native-"+rand.Text()+"?mode=memory&cache=shared&", 2)
		if err != nil {
			return nil, errors.Join(err, b.Close())
		}
		b.dbs[kind] = db
		keeper, err := db.Conn(ctx)
		if err != nil {
			return nil, errors.Join(err, b.Close())
		}
		b.keepers[kind] = keeper
		if err = reset(ctx, db, kind, SchemaVersion); err != nil {
			return nil, errors.Join(err, b.Close())
		}
	}
	return b, nil
}

func (b *Bundle) Database(kind Kind) (*sql.DB, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	db := b.dbs[kind]
	if db == nil {
		return nil, errors.New("invalid_native_index_kind")
	}
	return db, nil
}

// The containing Context drains leases/tasks first. Close attempts every handle
// even when one fails, and repeated Close returns the original result.
func (b *Bundle) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return b.closeErr
	}
	b.closed = true
	for _, kind := range []Kind{Main, History, Assets, BlockTree} {
		if c := b.keepers[kind]; c != nil {
			b.closeErr = errors.Join(b.closeErr, c.Close())
		}
		if db := b.dbs[kind]; db != nil {
			b.closeErr = errors.Join(b.closeErr, db.Close())
		}
	}
	clear(b.keepers)
	clear(b.dbs)
	return b.closeErr
}
