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

package bazaar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"wiki-kernel/internal/catalogue"

	"github.com/88250/gulu"
	"github.com/araddon/dateparse"
	"github.com/imroc/req/v3"
	gcache "github.com/patrickmn/go-cache"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/httpclient"
	"github.com/siyuan-note/logging"
	"golang.org/x/mod/semver"
	textUnicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"wiki-kernel/util"
)

// BackgroundScheduler binds non-authoritative follow-up work to an explicit
// lifecycle owner. Package downloads never choose a process owner themselves.
type BackgroundScheduler func(func(context.Context)) bool

type DisplayName = catalogue.DisplayName
type Description = catalogue.Description
type Readme = catalogue.Readme
type Funding = catalogue.Funding
type Package = catalogue.Package
type StagePackage = catalogue.StagePackage
type StageRepo = catalogue.StageRepo
type StageIndex = catalogue.StageIndex

func getPreferredReadme(p *Readme) string { return catalogue.PreferredReadme(p, util.Lang) }

func GetPreferredName(p *Package) string { return catalogue.PreferredName(p, util.Lang) }

func getPreferredDesc(p *Description) string { return catalogue.PreferredDescription(p, util.Lang) }

func getPreferredFunding(p *Funding) string { return catalogue.PreferredFunding(p) }

func PluginJSON(pluginDirName string) (ret *Plugin, err error) {
	p := filepath.Join(util.DataDir, "plugins", pluginDirName, "plugin.json")
	if !filelock.IsExist(p) {
		err = os.ErrNotExist
		return
	}
	data, err := filelock.ReadFile(p)
	if err != nil {
		logging.LogErrorf("read plugin.json [%s] failed: %s", p, err)
		return
	}
	if err = gulu.JSON.UnmarshalJSON(data, &ret); err != nil {
		logging.LogErrorf("parse plugin.json [%s] failed: %s", p, err)
		return
	}

	ret.URL = strings.TrimSuffix(ret.URL, "/")
	return
}

func WidgetJSON(widgetDirName string) (ret *Widget, err error) {
	p := filepath.Join(util.DataDir, "widgets", widgetDirName, "widget.json")
	if !filelock.IsExist(p) {
		err = os.ErrNotExist
		return
	}
	data, err := filelock.ReadFile(p)
	if err != nil {
		logging.LogErrorf("read widget.json [%s] failed: %s", p, err)
		return
	}
	if err = gulu.JSON.UnmarshalJSON(data, &ret); err != nil {
		logging.LogErrorf("parse widget.json [%s] failed: %s", p, err)
		return
	}

	ret.URL = strings.TrimSuffix(ret.URL, "/")
	return
}

func IconJSON(iconDirName string) (ret *Icon, err error) {
	p := filepath.Join(util.IconsPath, iconDirName, "icon.json")
	if !gulu.File.IsExist(p) {
		err = os.ErrNotExist
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		logging.LogErrorf("read icon.json [%s] failed: %s", p, err)
		return
	}
	if err = gulu.JSON.UnmarshalJSON(data, &ret); err != nil {
		logging.LogErrorf("parse icon.json [%s] failed: %s", p, err)
		return
	}

	ret.URL = strings.TrimSuffix(ret.URL, "/")
	return
}

func TemplateJSON(templateDirName string) (ret *Template, err error) {
	p := filepath.Join(util.DataDir, "templates", templateDirName, "template.json")
	if !filelock.IsExist(p) {
		err = os.ErrNotExist
		return
	}
	data, err := filelock.ReadFile(p)
	if err != nil {
		logging.LogErrorf("read template.json [%s] failed: %s", p, err)
		return
	}
	if err = gulu.JSON.UnmarshalJSON(data, &ret); err != nil {
		logging.LogErrorf("parse template.json [%s] failed: %s", p, err)
		return
	}

	ret.URL = strings.TrimSuffix(ret.URL, "/")
	return
}

func ThemeJSON(themeDirName string) (ret *Theme, err error) {
	p := filepath.Join(util.ThemesPath, themeDirName, "theme.json")
	if !gulu.File.IsExist(p) {
		err = os.ErrNotExist
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		logging.LogErrorf("read theme.json [%s] failed: %s", p, err)
		return
	}

	ret = &Theme{}
	if err = gulu.JSON.UnmarshalJSON(data, &ret); err != nil {
		logging.LogErrorf("parse theme.json [%s] failed: %s", p, err)
		return
	}

	ret.URL = strings.TrimSuffix(ret.URL, "/")
	return
}

var cachedStageIndex = map[string]*StageIndex{}
var stageIndexCacheTime int64
var stageIndexLock = sync.Mutex{}

func getStageIndex(pkgType string) (ret *StageIndex, err error) {
	rhyRet, err := util.GetRhyResult(false)
	if err != nil {
		return
	}

	stageIndexLock.Lock()
	defer stageIndexLock.Unlock()

	now := time.Now().Unix()
	if 3600 >= now-stageIndexCacheTime && nil != cachedStageIndex[pkgType] {
		ret = cachedStageIndex[pkgType]
		return
	}

	bazaarHash := rhyRet["bazaar"].(string)
	ret = &StageIndex{}
	request := httpclient.NewBrowserRequest()
	u := util.BazaarOSSServer + "/bazaar@" + bazaarHash + "/stage/" + pkgType + ".json"
	resp, reqErr := request.SetSuccessResult(ret).Get(u)
	if nil != reqErr {
		logging.LogErrorf("get community stage index [%s] failed: %s", u, reqErr)
		return
	}
	if 200 != resp.StatusCode {
		logging.LogErrorf("get community stage index [%s] failed: %d", u, resp.StatusCode)
		return
	}

	stageIndexCacheTime = now
	cachedStageIndex[pkgType] = ret
	return
}

func isOutdatedTheme(theme *Theme, bazaarThemes []*Theme) bool {
	if !strings.HasPrefix(theme.URL, "https://github.com/") {
		return false
	}

	repo := strings.TrimPrefix(theme.URL, "https://github.com/")
	parts := strings.Split(repo, "/")
	if 2 != len(parts) || "" == strings.TrimSpace(parts[1]) {
		return false
	}

	for _, pkg := range bazaarThemes {
		if theme.URL == pkg.URL && theme.Name == pkg.Name && 0 > semver.Compare("v"+theme.Version, "v"+pkg.Version) {
			theme.RepoHash = pkg.RepoHash
			return true
		}
	}
	return false
}

func isOutdatedIcon(icon *Icon, bazaarIcons []*Icon) bool {
	if !strings.HasPrefix(icon.URL, "https://github.com/") {
		return false
	}

	repo := strings.TrimPrefix(icon.URL, "https://github.com/")
	parts := strings.Split(repo, "/")
	if 2 != len(parts) || "" == strings.TrimSpace(parts[1]) {
		return false
	}

	for _, pkg := range bazaarIcons {
		if icon.URL == pkg.URL && icon.Name == pkg.Name && 0 > semver.Compare("v"+icon.Version, "v"+pkg.Version) {
			icon.RepoHash = pkg.RepoHash
			return true
		}
	}
	return false
}

func isOutdatedPlugin(plugin *Plugin, bazaarPlugins []*Plugin) bool {
	if !strings.HasPrefix(plugin.URL, "https://github.com/") {
		return false
	}

	repo := strings.TrimPrefix(plugin.URL, "https://github.com/")
	parts := strings.Split(repo, "/")
	if 2 != len(parts) || "" == strings.TrimSpace(parts[1]) {
		return false
	}

	for _, pkg := range bazaarPlugins {
		if plugin.URL == pkg.URL && plugin.Name == pkg.Name && 0 > semver.Compare("v"+plugin.Version, "v"+pkg.Version) {
			plugin.RepoHash = pkg.RepoHash
			return true
		}
	}
	return false
}

func isOutdatedWidget(widget *Widget, bazaarWidgets []*Widget) bool {
	if !strings.HasPrefix(widget.URL, "https://github.com/") {
		return false
	}

	repo := strings.TrimPrefix(widget.URL, "https://github.com/")
	parts := strings.Split(repo, "/")
	if 2 != len(parts) || "" == strings.TrimSpace(parts[1]) {
		return false
	}

	for _, pkg := range bazaarWidgets {
		if widget.URL == pkg.URL && widget.Name == pkg.Name && 0 > semver.Compare("v"+widget.Version, "v"+pkg.Version) {
			widget.RepoHash = pkg.RepoHash
			return true
		}
	}
	return false
}

func isOutdatedTemplate(template *Template, bazaarTemplates []*Template) bool {
	if !strings.HasPrefix(template.URL, "https://github.com/") {
		return false
	}

	repo := strings.TrimPrefix(template.URL, "https://github.com/")
	parts := strings.Split(repo, "/")
	if 2 != len(parts) || "" == strings.TrimSpace(parts[1]) {
		return false
	}

	for _, pkg := range bazaarTemplates {
		if template.URL == pkg.URL && template.Name == pkg.Name && 0 > semver.Compare("v"+template.Version, "v"+pkg.Version) {
			template.RepoHash = pkg.RepoHash
			return true
		}
	}
	return false
}

func isBazzarOnline() (ret bool) {
	// Improve marketplace loading when offline https://github.com/siyuan-note/siyuan/issues/12050
	ret = util.IsOnline(util.BazaarOSSServer, true, 3000)
	if !ret {
		util.PushErrMsg(util.Langs[util.Lang][24], 5000)
	}
	return
}

func GetPackageREADME(repoURL, repoHash, packageType string) (ret string) {
	repoURLHash := repoURL + "@" + repoHash

	stageIndex := cachedStageIndex[packageType]
	if nil == stageIndex {
		return
	}

	url := strings.TrimPrefix(repoURLHash, "https://github.com/")
	var repo *StageRepo
	for _, r := range stageIndex.Repos {
		if r.URL == url {
			repo = r
			break
		}
	}
	if nil == repo {
		return
	}

	readme := getPreferredReadme(repo.Package.Readme)

	data, err := downloadPackage(repoURLHash+"/"+readme, false, "")
	if err != nil {
		ret = fmt.Sprintf("Load bazaar package's README.md(%s) failed: %s", readme, err.Error())
		if readme == repo.Package.Readme.Default || "" == strings.TrimSpace(repo.Package.Readme.Default) {
			return
		}
		readme = repo.Package.Readme.Default
		data, err = downloadPackage(repoURLHash+"/"+readme, false, "")
		if err != nil {
			ret += fmt.Sprintf("<br>Load bazaar package's README.md(%s) failed: %s", readme, err.Error())
			return
		}
	}

	if 2 < len(data) {
		if 255 == data[0] && 254 == data[1] {
			data, _, err = transform.Bytes(textUnicode.UTF16(textUnicode.LittleEndian, textUnicode.ExpectBOM).NewDecoder(), data)
		} else if 254 == data[0] && 255 == data[1] {
			data, _, err = transform.Bytes(textUnicode.UTF16(textUnicode.BigEndian, textUnicode.ExpectBOM).NewDecoder(), data)
		}
	}

	ret, err = renderREADME(repoURL, data)
	return
}

func renderREADME(repoURL string, raw []byte) (string, error) {
	return catalogue.RenderREADME("https://cdn.jsdelivr.net/gh/"+strings.TrimPrefix(repoURL, "https://github.com/"), raw)
}

func renderLocalREADME(base string, raw []byte) (string, error) {
	return catalogue.RenderREADME(base, raw)
}

var (
	packageLocks     = map[string]*sync.Mutex{}
	packageLocksLock = sync.Mutex{}
)

func downloadPackage(repoURLHash string, pushProgress bool, systemID string) (data []byte, err error) {
	return downloadPackageContext(context.Background(), repoURLHash, pushProgress, systemID)
}

func downloadPackageContext(ctx context.Context, repoURLHash string, pushProgress bool, systemID string, schedulers ...BackgroundScheduler) (data []byte, err error) {
	if ctx == nil {
		return nil, context.Canceled
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	packageLocksLock.Lock()

	// repoURLHash: https://github.com/88250/Comfortably-Numb@6286912c381ef3f83e455d06ba4d369c498238dc
	separator := strings.LastIndex(repoURLHash, "@")
	if separator < 1 || separator == len(repoURLHash)-1 {
		packageLocksLock.Unlock()
		return nil, errors.New("invalid bazaar package identity")
	}
	repoURL := repoURLHash[:separator]
	lock, ok := packageLocks[repoURLHash]
	if !ok {
		lock = &sync.Mutex{}
		packageLocks[repoURLHash] = lock
	}
	packageLocksLock.Unlock()
	lock.Lock()
	defer lock.Unlock()
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	repoURLHash = strings.TrimPrefix(repoURLHash, "https://github.com/")
	u := util.BazaarOSSServer + "/package/" + repoURLHash
	buf := &bytes.Buffer{}
	resp, err := httpclient.NewCloudFileRequest2m().SetContext(ctx).SetOutput(buf).SetDownloadCallback(func(info req.DownloadInfo) {
		if pushProgress && ctx.Err() == nil && info.Response.ContentLength > 0 {
			progress := float32(info.DownloadedSize) / float32(info.Response.ContentLength)
			//logging.LogDebugf("downloading bazaar package [%f]", progress)
			util.PushDownloadProgress(repoURL, progress)
		}
	}).Get(u)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		logging.LogErrorf("get bazaar package [%s] failed: %s", u, err)
		return nil, errors.New("get bazaar package failed, please check your network")
	}
	if 200 != resp.StatusCode {
		logging.LogErrorf("get bazaar package [%s] failed: %d", u, resp.StatusCode)
		return nil, errors.New("get bazaar package failed: " + resp.Status)
	}
	data = buf.Bytes()
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	schedulePackageDownloadCount(repoURLHash, systemID, schedulers...)
	return
}

type packageDownloadCount struct {
	repoURLHash string
	systemID    string
	cloudServer string
}

func schedulePackageDownloadCount(repoURLHash, systemID string, schedulers ...BackgroundScheduler) bool {
	if strings.Contains(repoURLHash, ".md") || "" == systemID {
		return false
	}
	if len(schedulers) == 0 || schedulers[0] == nil {
		return false
	}
	count := packageDownloadCount{
		repoURLHash: repoURLHash,
		systemID:    systemID,
		cloudServer: util.GetCloudServer(),
	}
	return schedulers[0](func(ctx context.Context) {
		if err := incPackageDownloadsContext(ctx, count); err != nil && !errors.Is(err, context.Canceled) {
			logging.LogDebugf("increase bazaar package download count failed: %s", err)
		}
	})
}

func incPackageDownloadsContext(ctx context.Context, count packageDownloadCount) error {
	if ctx == nil {
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	repo := strings.Split(count.repoURLHash, "@")[0]
	u := count.cloudServer + "/apis/siyuan/bazaar/addBazaarPackageDownloadCount"
	_, err := httpclient.NewCloudRequest30s().SetContext(ctx).SetBody(
		map[string]interface{}{
			"systemID": count.systemID,
			"repo":     repo,
		}).Post(u)
	return err
}

func uninstallPackage(installPath string) (err error) {
	return uninstallPackageContext(context.Background(), installPath)
}

func uninstallPackageContext(ctx context.Context, installPath string) (err error) {
	if ctx == nil {
		return context.Canceled
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.RemoveAll(installPath); err != nil {
		logging.LogErrorf("remove [%s] failed: %s", installPath, err)
		return fmt.Errorf("remove community package [%s] failed", filepath.Base(installPath))
	}
	packageCache.Flush()
	return ctx.Err()
}

func installPackage(data []byte, installPath, repoURLHash string) (err error) {
	return installPackageContext(context.Background(), data, installPath, repoURLHash)
}

func installPackageContext(ctx context.Context, data []byte, installPath, repoURLHash string) (err error) {
	err = installPackage0Context(ctx, data, installPath)
	if err != nil {
		return
	}

	packageCache.Delete(strings.TrimPrefix(repoURLHash, "https://github.com/"))
	return ctx.Err()
}

func installPackage0(data []byte, installPath string) (err error) {
	return installPackage0Context(context.Background(), data, installPath)
}

func installPackage0Context(ctx context.Context, data []byte, installPath string) (err error) {
	if ctx == nil {
		return context.Canceled
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	tmpPackage := filepath.Join(util.TempDir, "bazaar", "package")
	if err = os.MkdirAll(tmpPackage, 0755); err != nil {
		return
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	name := gulu.Rand.String(7)
	tmp := filepath.Join(tmpPackage, name+".zip")
	if err = os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	if err = ctx.Err(); err != nil {
		return err
	}

	unzipPath := filepath.Join(tmpPackage, name)
	if err = gulu.Zip.Unzip(tmp, unzipPath); err != nil {
		logging.LogErrorf("write file [%s] failed: %s", installPath, err)
		return
	}
	if err = ctx.Err(); err != nil {
		return err
	}

	dirs, err := os.ReadDir(unzipPath)
	if err != nil {
		return
	}

	srcPath := unzipPath
	if 1 == len(dirs) && dirs[0].IsDir() {
		srcPath = filepath.Join(unzipPath, dirs[0].Name())
	}

	if err = ctx.Err(); err != nil {
		return err
	}
	if err = filelock.Copy(srcPath, installPath); err != nil {
		return
	}
	return nil
}

func formatUpdated(updated string) (ret string) {
	t, e := dateparse.ParseIn(updated, time.Now().Location())
	if nil == e {
		ret = t.Format("2006-01-02")
	} else {
		if strings.Contains(updated, "T") {
			ret = updated[:strings.Index(updated, "T")]
		} else {
			ret = strings.ReplaceAll(strings.ReplaceAll(updated, "T", ""), "Z", "")
		}
	}
	return
}

type bazaarPackage struct {
	Name      string `json:"name"`
	Downloads int    `json:"downloads"`
}

var cachedBazaarIndex = map[string]*bazaarPackage{}
var bazaarIndexCacheTime int64
var bazaarIndexLock = sync.Mutex{}

func getBazaarIndex() map[string]*bazaarPackage {
	bazaarIndexLock.Lock()
	defer bazaarIndexLock.Unlock()

	now := time.Now().Unix()
	if 3600 >= now-bazaarIndexCacheTime {
		return cachedBazaarIndex
	}

	request := httpclient.NewBrowserRequest()
	u := util.BazaarStatServer + "/bazaar/index.json"
	resp, reqErr := request.SetSuccessResult(&cachedBazaarIndex).Get(u)
	if nil != reqErr {
		logging.LogErrorf("get bazaar index [%s] failed: %s", u, reqErr)
		return cachedBazaarIndex
	}
	if 200 != resp.StatusCode {
		logging.LogErrorf("get bazaar index [%s] failed: %d", u, resp.StatusCode)
		return cachedBazaarIndex
	}
	bazaarIndexCacheTime = now
	return cachedBazaarIndex
}

// defaultMinAppVersion 如果集市包中缺失 minAppVersion 项，则使用该值作为最低支持的版本号，小于该版本号时不显示集市包
// Add marketplace package config item `minAppVersion` https://github.com/siyuan-note/siyuan/issues/8330
const defaultMinAppVersion = "2.9.0"

func disallowDisplayBazaarPackage(pkg *Package) bool {
	if "" == pkg.MinAppVersion {
		pkg.MinAppVersion = defaultMinAppVersion
	}

	if 0 < semver.Compare("v"+pkg.MinAppVersion, "v"+util.Ver) {
		return true
	}
	return false
}

// Retained only for the old package-level compatibility exports. The active
// catalogue has an AppConf-owned Service; these legacy byte caches start no
// import-time janitor and are explicitly invalidated by their old clear hook.
var packageCache = gcache.New(6*time.Hour, 0) // [repoURL]*Package

func CleanBazaarPackageCache() {
	packageCache.Flush()
	packageInstallSizeCache.Flush()
}

var packageInstallSizeCache = gcache.New(48*time.Hour, 0) // [repoURL]*int64
