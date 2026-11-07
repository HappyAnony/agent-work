// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package catalogue

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/go-humanize"
	"github.com/araddon/dateparse"
	"golang.org/x/mod/semver"
	textUnicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
	"wiki-kernel/internal/extensionprefs"
)

var ErrDependency = errors.New("catalogue_dependencies_unbound")
var ErrValue = errors.New("catalogue_value_invalid")

const MaxBytes = 16 << 20

type Options struct {
	Language, Version, Backend, OSS, Statistics, VersionURL string
	CurrentIcon, LightTheme, DarkTheme                      string
	ThemeNames                                              []string
	PluginEnabled                                           map[string]bool
	Fetch                                                   func(context.Context, string) ([]byte, error)
	Read                                                    func(context.Context, string, string, string) ([]byte, error)
	List                                                    func(context.Context, string, string) ([]fs.DirEntry, error)
	Stat                                                    func(context.Context, string, string, string) (fs.FileInfo, error)
	Now                                                     func() time.Time
}
type Request struct {
	Action, Kind, Frontend, Keyword, RepoURL, RepoHash string
	CheckUpdate                                        bool
}
type Item struct {
	*Package
	Modes   []string `json:"modes,omitempty"`
	Enabled bool     `json:"enabled,omitempty"`
}
type Result struct {
	Items  []*Item
	README string
}
type cached struct {
	raw   []byte
	until time.Time
}
type Service struct {
	once    sync.Once
	gate    chan struct{}
	cacheMu sync.Mutex
	cache   map[string]cached
	bytes   int
	epoch   uint64
}

func (s *Service) Clear() {
	if s == nil {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache = map[string]cached{}
	s.bytes = 0
	s.epoch++
}

func (s *Service) Do(ctx context.Context, q Request, p Options) (out Result, err error) {
	if ctx == nil || s == nil || p.Now == nil {
		return out, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if !validKind(q.Kind) || len(q.Keyword) > 4096 {
		return out, ErrValue
	}
	s.once.Do(func() { s.gate = make(chan struct{}, 1); s.cache = map[string]cached{} })
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return out, ctx.Err()
	}
	switch q.Action {
	case "remote":
		out.Items, err = s.remote(ctx, q, p)
	case "installed", "search":
		out.Items, err = s.installed(ctx, q, p)
	case "readme":
		out.README, err = s.readme(ctx, q, p)
	default:
		err = ErrValue
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return Result{}, err
	}
	return
}
func validKind(k string) bool {
	switch k {
	case "plugins", "widgets", "icons", "themes", "templates":
		return true
	}
	return false
}
func validName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\\x00")
}
func filename(k string) string { return strings.TrimSuffix(k, "s") + ".json" }
func (s *Service) fetch(ctx context.Context, p Options, u string) ([]byte, error) {
	if p.Fetch == nil {
		return nil, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	s.cacheMu.Lock()
	epoch := s.epoch
	old, ok := s.cache[u]
	if ok && p.Now().Before(old.until) {
		out := append([]byte(nil), old.raw...)
		s.cacheMu.Unlock()
		return out, nil
	}
	s.cacheMu.Unlock()
	raw, e := p.Fetch(ctx, u)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if len(raw) > MaxBytes {
		return nil, ErrValue
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if epoch != s.epoch {
		return raw, nil
	}
	if s.bytes+len(raw) > 64<<20 || len(s.cache) > 20000 {
		s.cache = map[string]cached{}
		s.bytes = 0
	}
	if old, ok := s.cache[u]; ok {
		s.bytes -= len(old.raw)
	}
	s.cache[u] = cached{append([]byte(nil), raw...), p.Now().Add(time.Hour)}
	s.bytes += len(raw)
	return raw, nil
}
func (s *Service) stage(ctx context.Context, q Request, p Options) (*StageIndex, error) {
	if p.OSS == "" || p.VersionURL == "" {
		return nil, ErrDependency
	}
	raw, e := s.fetch(ctx, p, p.VersionURL)
	if e != nil {
		return nil, e
	}
	var version struct {
		Bazaar string `json:"bazaar"`
	}
	if e = json.Unmarshal(raw, &version); e != nil {
		return nil, e
	}
	if !validName(version.Bazaar) {
		return nil, ErrValue
	}
	raw, e = s.fetch(ctx, p, strings.TrimSuffix(p.OSS, "/")+"/bazaar@"+url.PathEscape(version.Bazaar)+"/stage/"+q.Kind+".json")
	if e != nil {
		return nil, e
	}
	var index StageIndex
	if e = json.Unmarshal(raw, &index); e != nil {
		return nil, e
	}
	if len(index.Repos) > 10000 {
		return nil, ErrValue
	}
	return &index, nil
}
func decode(raw []byte) (*Item, error) {
	if len(raw) > MaxBytes {
		return nil, ErrValue
	}
	var item *Item
	if e := json.Unmarshal(raw, &item); e != nil {
		return nil, e
	}
	if item == nil || item.Package == nil || !validName(item.Name) {
		return nil, ErrValue
	}
	item.URL = strings.TrimSuffix(item.URL, "/")
	return item, nil
}
func repoParts(identity string) (string, string, error) {
	if strings.ContainsAny(identity, "?#\x00") {
		return "", "", ErrValue
	}
	parts := strings.Split(identity, "@")
	if len(parts) != 2 || !validName(parts[1]) {
		return "", "", ErrValue
	}
	repo := strings.Split(parts[0], "/")
	if len(repo) != 2 || !validName(repo[0]) || !validName(repo[1]) {
		return "", "", ErrValue
	}
	return parts[0], parts[1], nil
}
func (s *Service) remote(ctx context.Context, q Request, p Options) ([]*Item, error) {
	if p.Version == "" || p.Statistics == "" {
		return nil, ErrDependency
	}
	index, e := s.stage(ctx, q, p)
	if e != nil {
		return nil, e
	}
	raw, e := s.fetch(ctx, p, strings.TrimSuffix(p.Statistics, "/")+"/bazaar/index.json")
	if e != nil {
		return nil, e
	}
	downloads := map[string]struct {
		Downloads int `json:"downloads"`
	}{}
	if e = json.Unmarshal(raw, &downloads); e != nil {
		return nil, e
	}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	items := make([]*Item, len(index.Repos))
	jobs := make(chan int)
	var group sync.WaitGroup
	var first error
	var fail sync.Once
	for i := 0; i < min(8, len(index.Repos)); i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for n := range jobs {
				if work.Err() != nil {
					continue
				}
				repo := index.Repos[n]
				item, e := func() (*Item, error) {
					if repo == nil || repo.Package == nil || repo.Size < 0 || repo.InstallSize < 0 {
						return nil, ErrValue
					}
					name, hash, e := repoParts(repo.URL)
					if e != nil {
						return nil, e
					}
					raw, e := s.fetch(work, p, strings.TrimSuffix(p.OSS, "/")+"/package/"+repo.URL+"/"+filename(q.Kind))
					if e != nil {
						return nil, e
					}
					item, e := decode(raw)
					if e != nil {
						return nil, e
					}
					if item.MinAppVersion == "" {
						item.MinAppVersion = "2.9.0"
					}
					if semver.Compare("v"+item.MinAppVersion, "v"+p.Version) > 0 {
						return nil, nil
					}
					item.RepoURL = "https://github.com/" + name
					item.RepoHash = hash
					base := strings.TrimSuffix(p.OSS, "/") + "/package/" + repo.URL
					item.PreviewURL = base + "/preview.png?imageslim"
					item.PreviewURLThumb = base + "/preview.png?imageView2/2/w/436/h/232"
					item.IconURL = base + "/icon.png"
					item.Funding = repo.Package.Funding
					item.PreferredFunding = PreferredFunding(item.Funding)
					item.PreferredName = PreferredName(item.Package, p.Language)
					item.PreferredDesc = PreferredDescription(item.Description, p.Language)
					item.Updated, item.Stars, item.OpenIssues = repo.Updated, repo.Stars, repo.OpenIssues
					item.Size, item.InstallSize = repo.Size, repo.InstallSize
					item.HSize = humanize.BytesCustomCeil(uint64(item.Size), 2)
					item.HInstallSize = humanize.BytesCustomCeil(uint64(item.InstallSize), 2)
					if t, e := dateparse.ParseIn(repo.Updated, p.Now().Location()); e == nil {
						item.HUpdated = t.Format("2006-01-02")
					} else {
						item.HUpdated = strings.Split(repo.Updated, "T")[0]
					}
					item.Downloads = downloads[name].Downloads
					item.Incompatible = q.Kind == "plugins" && extensionprefs.Incompatible(extensionprefs.Package{Backends: item.Backends, Frontends: item.Frontends}, p.Backend, q.Frontend)
					if p.Stat == nil || p.Read == nil {
						return nil, ErrDependency
					}
					installed := false
					if q.Kind == "themes" {
						for _, name := range p.ThemeNames {
							if name == item.Name {
								installed = true
								break
							}
						}
					} else {
						info, e := p.Stat(work, q.Kind, item.Name, ".")
						if e == nil {
							installed = info.IsDir()
						} else if !absent(e) {
							return nil, e
						}
					}
					item.Installed = installed
					item.Current = q.Kind == "icons" && item.Name == p.CurrentIcon || q.Kind == "themes" && (item.Name == p.LightTheme || item.Name == p.DarkTheme)
					if installed {
						local, e := p.Read(work, q.Kind, item.Name, filename(q.Kind))
						if e != nil {
							return nil, e
						}
						got, e := decode(local)
						if e != nil {
							return nil, e
						}
						item.Outdated = semver.Compare("v"+got.Version, "v"+item.Version) < 0
					}
					if !Match(q.Keyword, item.Package) {
						return nil, nil
					}
					return item, nil
				}()
				if e != nil {
					fail.Do(func() { first = e; cancel() })
					continue
				}
				items[n] = item
			}
		}()
	}
	for n := range index.Repos {
		select {
		case jobs <- n:
		case <-work.Done():
			break
		}
	}
	close(jobs)
	group.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if first != nil {
		return nil, first
	}
	out := []*Item{}
	for _, item := range items {
		if item != nil {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out, nil
}

func absent(e error) bool {
	if e == nil {
		return false
	}
	if many, ok := e.(interface{ Unwrap() []error }); ok {
		causes := many.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !absent(cause) {
				return false
			}
		}
		return true
	}
	return errors.Is(e, fs.ErrNotExist)
}
func publicBase(kind, name string) string {
	if kind == "themes" || kind == "icons" {
		return "/appearance/" + kind + "/" + name + "/"
	}
	return "/" + kind + "/" + name + "/"
}
func builtin(kind, name string) bool {
	return kind == "themes" && (name == "daylight" || name == "midnight") || kind == "icons" && (name == "ant" || name == "material")
}
func (s *Service) installed(ctx context.Context, q Request, p Options) (out []*Item, err error) {
	if p.List == nil || p.Read == nil || p.Stat == nil {
		return nil, ErrDependency
	}
	out = []*Item{}
	entries, e := p.List(ctx, q.Kind, "")
	if absent(e) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	var remote []*Item
	if q.CheckUpdate && q.Action != "search" {
		remote, e = s.remote(ctx, Request{Action: "remote", Kind: q.Kind, Frontend: q.Frontend}, p)
		if e != nil {
			return nil, e
		}
	}
	for _, entry := range entries {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		name := entry.Name()
		if !validName(name) || strings.HasPrefix(name, ".") || builtin(q.Kind, name) {
			continue
		}
		info, e := p.Stat(ctx, q.Kind, name, ".")
		if e != nil {
			return nil, e
		}
		if !info.IsDir() {
			continue
		}
		raw, e := p.Read(ctx, q.Kind, name, filename(q.Kind))
		if absent(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		item, e := decode(raw)
		if e != nil {
			return nil, e
		}
		if !Match(q.Keyword, item.Package) {
			continue
		}
		if q.Action == "search" {
			item.PreferredName = PreferredName(item.Package, p.Language)
			out = append(out, item)
			continue
		}
		item.Installed = true
		item.RepoURL = item.URL
		base := publicBase(q.Kind, name)
		item.PreviewURL = base + "preview.png"
		item.PreviewURLThumb = item.PreviewURL
		item.IconURL = base + "icon.png"
		item.PreferredFunding = PreferredFunding(item.Funding)
		item.PreferredName = PreferredName(item.Package, p.Language)
		item.PreferredDesc = PreferredDescription(item.Description, p.Language)
		info, e = p.Stat(ctx, q.Kind, name, "README.md")
		if e != nil {
			return nil, e
		}
		item.HInstallDate = info.ModTime().Format("2006-01-02")
		item.InstallSize, e = directorySize(ctx, q.Kind, name, ".", p)
		if e != nil {
			return nil, e
		}
		item.HInstallSize = humanize.BytesCustomCeil(uint64(item.InstallSize), 2)
		readme := PreferredReadme(item.Readme, p.Language)
		if !fs.ValidPath(readme) || strings.Contains(readme, "\\") {
			return nil, ErrValue
		}
		raw, e = p.Read(ctx, q.Kind, name, readme)
		if e != nil {
			return nil, e
		}
		item.PreferredReadme, e = RenderREADME(base, raw)
		if e != nil {
			return nil, e
		}
		for _, latest := range remote {
			if latest.Name == item.Name && latest.URL == item.URL && strings.HasPrefix(item.URL, "https://github.com/") && semver.Compare("v"+item.Version, "v"+latest.Version) < 0 {
				item.Outdated = true
				item.RepoHash = latest.RepoHash
				break
			}
		}
		item.Incompatible = q.Kind == "plugins" && extensionprefs.Incompatible(extensionprefs.Package{Backends: item.Backends, Frontends: item.Frontends}, p.Backend, q.Frontend)
		item.Enabled = p.PluginEnabled[item.Name]
		item.Current = q.Kind == "icons" && item.Name == p.CurrentIcon || q.Kind == "themes" && (item.Name == p.LightTheme || item.Name == p.DarkTheme)
		out = append(out, item)
	}
	return out, ctx.Err()
}
func directorySize(ctx context.Context, kind, name, rel string, p Options) (int64, error) {
	type task struct {
		p     string
		depth int
	}
	pending := []task{{rel, 0}}
	var total int64
	count := 0
	for len(pending) > 0 {
		if e := ctx.Err(); e != nil {
			return 0, e
		}
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current.depth > 64 {
			return 0, ErrValue
		}
		entries, e := p.List(ctx, kind, path.Join(name, current.p))
		if e != nil {
			return 0, e
		}
		for _, entry := range entries {
			count++
			if count > 100000 {
				return 0, ErrValue
			}
			child := path.Join(current.p, entry.Name())
			info, e := p.Stat(ctx, kind, name, child)
			if e != nil {
				return 0, e
			}
			if info.IsDir() {
				pending = append(pending, task{child, current.depth + 1})
			} else {
				if !info.Mode().IsRegular() || info.Size() < 0 {
					return 0, ErrValue
				}
				total += info.Size()
			}
		}
	}
	return total, ctx.Err()
}
func (s *Service) readme(ctx context.Context, q Request, p Options) (string, error) {
	index, e := s.stage(ctx, q, p)
	if e != nil {
		return "", e
	}
	identity := strings.TrimPrefix(q.RepoURL, "https://github.com/") + "@" + q.RepoHash
	if _, _, e = repoParts(identity); e != nil {
		return "", e
	}
	var repo *StageRepo
	for _, r := range index.Repos {
		if r != nil && r.URL == identity {
			repo = r
			break
		}
	}
	if repo == nil || repo.Package == nil {
		return "", ErrValue
	}
	name := PreferredReadme(repo.Package.Readme, p.Language)
	fetch := func(name string) ([]byte, error) {
		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			return nil, ErrValue
		}
		return s.fetch(ctx, p, strings.TrimSuffix(p.OSS, "/")+"/package/"+identity+"/"+name)
	}
	raw, e := fetch(name)
	if e != nil && repo.Package.Readme != nil && name != repo.Package.Readme.Default && strings.TrimSpace(repo.Package.Readme.Default) != "" {
		raw, e = fetch(repo.Package.Readme.Default)
	}
	if e != nil {
		return "", e
	}
	if len(raw) > 2 {
		if raw[0] == 255 && raw[1] == 254 {
			raw, _, e = transform.Bytes(textUnicode.UTF16(textUnicode.LittleEndian, textUnicode.ExpectBOM).NewDecoder(), raw)
		} else if raw[0] == 254 && raw[1] == 255 {
			raw, _, e = transform.Bytes(textUnicode.UTF16(textUnicode.BigEndian, textUnicode.ExpectBOM).NewDecoder(), raw)
		}
	}
	if e != nil {
		return "", e
	}
	return RenderREADME("https://cdn.jsdelivr.net/gh/"+strings.TrimPrefix(q.RepoURL, "https://github.com/"), raw)
}
