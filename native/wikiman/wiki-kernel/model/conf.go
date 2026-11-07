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

package model

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/Xuanwo/go-locale"
	"github.com/sashabaranov/go-openai"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"golang.org/x/mod/semver"
	"golang.org/x/text/language"
	"wiki-kernel/conf"
	"wiki-kernel/internal/aiflow"
	"wiki-kernel/internal/catalogue"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/extensionprefs"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/pageprefs"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/sql"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

var Conf *AppConf

// AppConf 维护应用元数据，保存在 ~/.siyuan/conf.json。
type AppConf struct {
	conf.AppData

	// Single-notebook mode (runtime only, NOT persisted to conf.json).
	// Populated once by model.EnsureSingleNotebookAndDoc (M3) after InitBoxes;
	// read by api/system.go::getConf (M6) which manually projects them under
	// data.conf.system.* in the HTTP response. The `json:"-"` tag prevents the
	// generic Save() marshal from leaking these into conf.json on disk.
	SingleNotebookMode      bool               `json:"-"` // true iff kernel started with --single (or WIKI_SINGLE_NOTEBOOK=1)
	SingleNotebookBoxID     string             `json:"-"` // ID of the unique notebook; empty until EnsureSingleNotebookAndDoc completes
	SingleNotebookRootDocID string             `json:"-"` // ID of the unique root document; empty until EnsureSingleNotebookAndDoc completes
	UnmountedBoxes          []UnmountedBoxInfo `json:"-"` // legacy notebooks Unmounted during single-mode takeover (data preserved on disk)
	IgnoredDocs             []IgnoredDocInfo   `json:"-"` // additional root docs found under the unique notebook and ignored (data preserved)

	m                                sync.Mutex
	settingsGate                     chan struct{}
	customEmojis                     nativeparser.EmojiRegistry
	pagePreferences                  *pageprefs.Service
	extensionPreferences             *extensionprefs.Service
	catalogueResources               *catalogue.Service
	findReplaceGate                  *attributeMutationGate
	notebookMutationGate             *attributeMutationGate
	treeSearchAt                     time.Time
	cloudMu                          sync.Mutex
	cloudAccount                     *cloudaccount.Service
	cloudTasks                       *taskgroup.Group
	cloudClosed, cloudRefreshPending bool
	cloudReminderPending             bool
	cloudAnnouncementPending         bool
	syncRuntime                      legacySyncRuntime
	htmlExportMu                     sync.Mutex
	htmlExports                      map[string]htmlExportHandoff
	htmlExportsClosed                bool
	repoMu                           sync.Mutex
	repoOwner                        *cloudrepo.Service
	repoClosed                       bool
	repoTasks                        *taskgroup.Group
	repoDelayed                      []*repoDelayedTask
	archiveIDNoticeTail              <-chan struct{}
	repoReferenceCounts              map[string]*referenceCountTask
	historyGenerated                 sync.Map
	repoPurgePending                 bool
	repoCheckoutClosed               bool
	repoRestoreFailed                bool
	repoCheckouts                    map[string]*task.ContextTask
	aiMu                             sync.Mutex
	aiOwner                          *aiflow.Service
	aiClosed                         bool
	// Persistence authority is private, instance-owned, and never imported from JSON.
	confPath            string
	persistenceReadOnly bool
	business            *conf.Business
}

// UnmountedBoxInfo describes a notebook that existed in the workspace when single
// mode took over and was Unmounted (closed in-memory; disk data preserved).
// Emitted under data.conf.system.unmountedBoxes by /api/system/getConf when
// kernel runs with --single.
type UnmountedBoxInfo struct {
	ID     string `json:"id"`              // 14-char siyuan box ID
	Name   string `json:"name"`            // notebook display name as it appeared on disk
	Reason string `json:"reason"`          // "extra-box" | "unmount-failed"
	Error  string `json:"error,omitempty"` // present iff Reason == "unmount-failed"
}

// IgnoredDocInfo describes an additional root document found under the unique
// notebook (post-election) that single-mode silently ignored. Disk data is
// preserved. Emitted under data.conf.system.ignoredDocs by /api/system/getConf
// when kernel runs with --single.
type IgnoredDocInfo struct {
	BoxID string `json:"boxID"` // always equals AppConf.SingleNotebookBoxID
	ID    string `json:"id"`    // ignored root document ID
	Title string `json:"title"` // display title of the ignored document
}

func NewAppConf() *AppConf {
	return &AppConf{AppData: conf.AppData{LogLevel: "debug"}}
}

var ErrUnboundAppConf = errors.New("unbound_app_conf")

// NewAppConfAt binds the original complete configuration to an explicit owner.
// It does not read files, create directories or persist defaults.
func NewAppConfAt(confPath string, readOnly bool) (*AppConf, error) {
	if !filepath.IsAbs(confPath) {
		return nil, errors.New("app_conf_path_must_be_absolute")
	}
	ret := NewAppConf()
	ret.confPath = filepath.Clean(confPath)
	ret.persistenceReadOnly = readOnly
	return ret, nil
}

func (conf *AppConf) GetUILayout() *conf.UILayout {
	conf.m.Lock()
	defer conf.m.Unlock()
	return conf.UILayout
}

func (conf *AppConf) SetUILayout(uiLayout *conf.UILayout) {
	conf.m.Lock()
	defer conf.m.Unlock()
	conf.UILayout = uiLayout
}

func (c *AppConf) GetUser() *conf.User {
	c.m.Lock()
	defer c.m.Unlock()
	copy, _ := (conf.AppData{User: c.User}).Clone()
	return copy.User
}
func (c *AppConf) SetUser(user *conf.User) {
	c.m.Lock()
	defer c.m.Unlock()
	copy, e := (conf.AppData{User: user}).Clone()
	if e == nil {
		c.User = copy.User
	}
}

func InitConf() {
	initLang()

	confPath := filepath.Join(util.ConfDir, "conf.json")
	var err error
	Conf, err = NewAppConfAt(confPath, util.ReadOnly)
	if err != nil {
		panic(err) // boot must supply an absolute workspace configuration path
	}
	Conf.business = legacyConfigurationBusiness()
	if gulu.File.IsExist(confPath) {
		if data, err := os.ReadFile(confPath); err != nil {
			logging.LogErrorf("load conf [%s] failed: %s", confPath, err)
		} else {
			if err = gulu.JSON.UnmarshalJSON(data, Conf); err != nil {
				logging.LogErrorf("parse conf [%s] failed: %s", confPath, err)
			} else {
				logging.LogInfof("loaded conf [%s]", confPath)
			}
		}
	}

	if "" != util.Lang {
		initialized := false
		if util.ContainerAndroid == util.Container || util.ContainerIOS == util.Container || util.ContainerHarmony == util.Container {
			// 移动端以上次设置的外观语言为准
			if "" != Conf.Lang && util.Lang != Conf.Lang {
				util.Lang = Conf.Lang
				logging.LogInfof("use the last specified language [%s]", util.Lang)
				initialized = true
			}
		}

		if !initialized {
			Conf.Lang = util.Lang
			logging.LogInfof("initialized the specified language [%s]", util.Lang)
		}
	} else {
		if "" == Conf.Lang {
			// 未指定外观语言时使用系统语言

			if userLang, err := locale.Detect(); err == nil {
				var supportLangs []language.Tag
				for lang := range util.Langs {
					if tag, err := language.Parse(lang); err == nil {
						supportLangs = append(supportLangs, tag)
					} else {
						logging.LogErrorf("load language [%s] failed: %s", lang, err)
					}
				}
				matcher := language.NewMatcher(supportLangs)
				lang, _, _ := matcher.Match(userLang)
				base, _ := lang.Base()
				region, _ := lang.Region()
				util.Lang = base.String() + "_" + region.String()
				Conf.Lang = util.Lang
				logging.LogInfof("initialized language [%s] based on device locale", Conf.Lang)
			} else {
				logging.LogDebugf("check device locale failed [%s], using default language [en_US]", err)
				util.Lang = "en_US"
				Conf.Lang = util.Lang
			}
		}
		util.Lang = Conf.Lang
	}

	Conf.Langs = loadLangs()
	if nil == Conf.Appearance {
		Conf.Appearance = conf.NewAppearance()
	}
	var langOK bool
	for _, l := range Conf.Langs {
		if Conf.Lang == l.Name {
			langOK = true
			break
		}
	}
	if !langOK {
		Conf.Lang = "en_US"
		util.Lang = Conf.Lang
	}
	Conf.Appearance.Lang = Conf.Lang
	if nil == Conf.UILayout {
		Conf.UILayout = &conf.UILayout{}
	}
	if nil == Conf.Keymap {
		Conf.Keymap = &conf.Keymap{}
	}
	if "" == Conf.Appearance.CodeBlockThemeDark {
		Conf.Appearance.CodeBlockThemeDark = "dracula"
	}
	if "" == Conf.Appearance.CodeBlockThemeLight {
		Conf.Appearance.CodeBlockThemeLight = "github"
	}
	if nil == Conf.Appearance.StatusBar {
		Conf.Appearance.StatusBar = &util.StatusBar{}
	}
	util.StatusBarCfg = Conf.Appearance.StatusBar
	if nil == Conf.FileTree {
		Conf.FileTree = conf.NewFileTreeWithSettings(util.UseSingleLineSave, util.LargeFileWarningSize)
	}
	if 1 > Conf.FileTree.MaxListCount {
		Conf.FileTree.MaxListCount = 512
	}
	if 1 > Conf.FileTree.MaxOpenTabCount {
		Conf.FileTree.MaxOpenTabCount = 8
	}
	if 32 < Conf.FileTree.MaxOpenTabCount {
		Conf.FileTree.MaxOpenTabCount = 32
	}
	Conf.FileTree.DocCreateSavePath = util.TrimSpaceInPath(Conf.FileTree.DocCreateSavePath)
	Conf.FileTree.RefCreateSavePath = util.TrimSpaceInPath(Conf.FileTree.RefCreateSavePath)
	util.UseSingleLineSave = Conf.FileTree.UseSingleLineSave
	if 2 > Conf.FileTree.LargeFileWarningSize {
		Conf.FileTree.LargeFileWarningSize = 8
	}
	util.LargeFileWarningSize = Conf.FileTree.LargeFileWarningSize
	if nil == Conf.FileTree.CreateDocAtTop { // v3.4.0 之前的版本没有该字段，设置默认值为 true，即在顶部创建新文档，不改变用户习惯
		Conf.FileTree.CreateDocAtTop = func() *bool { b := true; return &b }()
	}

	util.CurrentCloudRegion = Conf.CloudRegion

	if nil == Conf.Tag {
		Conf.Tag = conf.NewTag()
	}

	defaultEditor := conf.NewEditorWithMarkdown(*util.MarkdownSettings)
	if nil == Conf.Editor {
		Conf.Editor = defaultEditor
	}
	// 新增字段的默认值
	// 使用指针类型来区分字段不存在（nil）和用户设置为 0（非 nil）
	if nil == Conf.Editor.BacklinkSort {
		Conf.Editor.BacklinkSort = defaultEditor.BacklinkSort
	}
	if nil == Conf.Editor.BackmentionSort {
		Conf.Editor.BackmentionSort = defaultEditor.BackmentionSort
	}
	if 1 > len(Conf.Editor.Emoji) {
		Conf.Editor.Emoji = []string{}
	}
	for i, emoji := range Conf.Editor.Emoji {
		if strings.Contains(emoji, ".") {
			// XSS through emoji name https://github.com/siyuan-note/siyuan/issues/15034
			emoji = util.FilterUploadEmojiFileName(emoji)
			Conf.Editor.Emoji[i] = emoji
		}
	}
	if 9 > Conf.Editor.FontSize || 72 < Conf.Editor.FontSize {
		Conf.Editor.FontSize = 16
	}
	if "" == Conf.Editor.PlantUMLServePath {
		Conf.Editor.PlantUMLServePath = "https://www.plantuml.com/plantuml/svg/~1"
	}
	if 1 > Conf.Editor.BlockRefDynamicAnchorTextMaxLen {
		Conf.Editor.BlockRefDynamicAnchorTextMaxLen = 64
	}
	if 5120 < Conf.Editor.BlockRefDynamicAnchorTextMaxLen {
		Conf.Editor.BlockRefDynamicAnchorTextMaxLen = 5120
	}
	if 1440 < Conf.Editor.GenerateHistoryInterval {
		Conf.Editor.GenerateHistoryInterval = 1440
	}
	if 1 > Conf.Editor.HistoryRetentionDays {
		Conf.Editor.HistoryRetentionDays = 30
	}
	if 3650 < Conf.Editor.HistoryRetentionDays {
		Conf.Editor.HistoryRetentionDays = 3650
	}
	if conf.MinDynamicLoadBlocks > Conf.Editor.DynamicLoadBlocks {
		Conf.Editor.DynamicLoadBlocks = conf.MinDynamicLoadBlocks
	}
	if 0 > Conf.Editor.BacklinkExpandCount {
		Conf.Editor.BacklinkExpandCount = 0
	}
	if -1 > Conf.Editor.BackmentionExpandCount {
		Conf.Editor.BackmentionExpandCount = -1
	}
	if nil == Conf.Editor.Markdown {
		Conf.Editor.Markdown = &util.Markdown{}
	}
	util.MarkdownSettings = Conf.Editor.Markdown

	if nil == Conf.Export {
		Conf.Export = conf.NewExport()
	}
	if 0 == Conf.Export.BlockRefMode || 1 == Conf.Export.BlockRefMode || 5 == Conf.Export.BlockRefMode {
		// 废弃导出选项引用块转换为原始块和引述块 https://github.com/siyuan-note/siyuan/issues/3155
		// 锚点哈希模式和脚注模式合并 https://github.com/siyuan-note/siyuan/issues/13331
		Conf.Export.BlockRefMode = 4 // 改为脚注+锚点哈希
	}
	if "" == Conf.Export.PandocBin {
		Conf.Export.PandocBin = util.PandocBinPath
	}

	if nil == Conf.Graph || nil == Conf.Graph.Local || nil == Conf.Graph.Global {
		Conf.Graph = conf.NewGraph()
	}

	if nil == Conf.System {
		Conf.System = conf.NewSystemForDevice(util.GetDeviceID(), util.GetDeviceName(), util.Ver)
		if util.ContainerIOS != util.Container {
			Conf.OpenHelp = true
		}
	} else {
		if 0 < semver.Compare("v"+util.Ver, "v"+Conf.System.KernelVersion) {
			logging.LogInfof("upgraded from version [%s] to [%s]", Conf.System.KernelVersion, util.Ver)
			Conf.ShowChangelog = true
		} else if 0 > semver.Compare("v"+util.Ver, "v"+Conf.System.KernelVersion) {
			logging.LogInfof("downgraded from version [%s] to [%s]", Conf.System.KernelVersion, util.Ver)
		}

		Conf.System.KernelVersion = util.Ver
		Conf.System.IsInsider = util.IsInsider
	}
	if nil == Conf.System.NetworkProxy {
		Conf.System.NetworkProxy = &conf.NetworkProxy{}
	}
	if "" == Conf.System.ID {
		Conf.System.ID = util.GetDeviceID()
	}
	if "" == Conf.System.Name {
		Conf.System.Name = util.GetDeviceName()
	}
	if util.ContainerStd == util.Container {
		Conf.System.ID = util.GetDeviceID()
		Conf.System.Name = util.GetDeviceName()
	}
	Conf.System.DisabledFeatures = util.DisabledFeatures
	if 1 > len(Conf.System.DisabledFeatures) {
		Conf.System.DisabledFeatures = []string{}
	}

	if nil == Conf.Snippet {
		Conf.Snippet = conf.NewSnpt()
	}

	Conf.System.AppDir = util.WorkingDir
	Conf.System.ConfDir = util.ConfDir
	Conf.System.HomeDir = util.HomeDir
	Conf.System.WorkspaceDir = util.WorkspaceDir
	Conf.System.DataDir = util.DataDir
	Conf.System.Container = util.Container
	Conf.System.IsMicrosoftStore = util.ISMicrosoftStore
	if util.ISMicrosoftStore {
		logging.LogInfof("using Microsoft Store edition")
	}
	Conf.System.OS = runtime.GOOS
	Conf.System.OSPlatform = util.GetOSPlatform()

	if "" != Conf.UserData {
		Conf.SetUser(loadUserFromConf())
	}
	if nil == Conf.Account {
		Conf.Account = conf.NewAccount()
	}

	if nil == Conf.Sync {
		Conf.Sync = conf.NewSync()
	}
	if 0 == Conf.Sync.Mode {
		Conf.Sync.Mode = 1
	}
	if 30 > Conf.Sync.Interval {
		Conf.Sync.Interval = 30
	}
	if 60*60*12 < Conf.Sync.Interval {
		Conf.Sync.Interval = 60 * 60 * 12
	}
	if nil == Conf.Sync.S3 {
		Conf.Sync.S3 = &conf.S3{PathStyle: true, SkipTlsVerify: true}
	}
	Conf.Sync.S3.Endpoint = util.NormalizeEndpoint(Conf.Sync.S3.Endpoint)
	Conf.Sync.S3.Timeout = util.NormalizeTimeout(Conf.Sync.S3.Timeout)
	Conf.Sync.S3.ConcurrentReqs = util.NormalizeConcurrentReqs(Conf.Sync.S3.ConcurrentReqs, conf.ProviderS3)
	if nil == Conf.Sync.WebDAV {
		Conf.Sync.WebDAV = &conf.WebDAV{SkipTlsVerify: true}
	}
	Conf.Sync.WebDAV.Endpoint = util.NormalizeEndpoint(Conf.Sync.WebDAV.Endpoint)
	Conf.Sync.WebDAV.Timeout = util.NormalizeTimeout(Conf.Sync.WebDAV.Timeout)
	Conf.Sync.WebDAV.ConcurrentReqs = util.NormalizeConcurrentReqs(Conf.Sync.WebDAV.ConcurrentReqs, conf.ProviderWebDAV)
	if nil == Conf.Sync.Local {
		Conf.Sync.Local = &conf.Local{}
	}
	Conf.Sync.Local.Endpoint = util.NormalizeLocalPath(Conf.Sync.Local.Endpoint)
	Conf.Sync.Local.Timeout = util.NormalizeTimeout(Conf.Sync.Local.Timeout)
	Conf.Sync.Local.ConcurrentReqs = util.NormalizeConcurrentReqs(Conf.Sync.Local.ConcurrentReqs, conf.ProviderLocal)

	if util.ContainerDocker == util.Container {
		Conf.Sync.Perception = false
	}

	if nil == Conf.Api {
		Conf.Api = conf.NewAPI()
	}

	if nil == Conf.Bazaar {
		Conf.Bazaar = conf.NewBazaar()
	}

	if nil == Conf.Publish {
		Conf.Publish = conf.NewPublish()
	}
	if Conf.OpenHelp && Conf.Publish.Enable {
		Conf.OpenHelp = false
	}

	if nil == Conf.Repo {
		Conf.Repo = conf.NewRepo()
	}
	if timingEnv := os.Getenv("WIKI_SYNC_INDEX_TIMING"); "" != timingEnv {
		val, err := strconv.Atoi(timingEnv)
		if err == nil {
			Conf.Repo.SyncIndexTiming = int64(val)
		}
	}
	if 12000 > Conf.Repo.SyncIndexTiming {
		Conf.Repo.SyncIndexTiming = 12 * 1000
	}
	if 1 > Conf.Repo.IndexRetentionDays {
		Conf.Repo.IndexRetentionDays = 180
	}
	if 1 > Conf.Repo.RetentionIndexesDaily {
		Conf.Repo.RetentionIndexesDaily = 2
	}
	if 0 < len(Conf.Repo.Key) {
		logging.LogInfof("repo key [%x]", sha1.Sum(Conf.Repo.Key))
	}

	if nil == Conf.Search {
		Conf.Search = conf.NewSearch()
	}
	if 1 > Conf.Search.Limit {
		Conf.Search.Limit = 64
	}
	if 32 > Conf.Search.Limit {
		Conf.Search.Limit = 32
	}
	if 1 > Conf.Search.BacklinkMentionKeywordsLimit {
		Conf.Search.BacklinkMentionKeywordsLimit = 512
	}

	if nil == Conf.Stat {
		Conf.Stat = conf.NewStat()
	}

	if nil == Conf.Flashcard {
		Conf.Flashcard = conf.NewFlashcard()
	}
	if 0 > Conf.Flashcard.NewCardLimit {
		Conf.Flashcard.NewCardLimit = 20
	}
	if 0 > Conf.Flashcard.ReviewCardLimit {
		Conf.Flashcard.ReviewCardLimit = 200
	}
	if 0 >= Conf.Flashcard.RequestRetention || 1 <= Conf.Flashcard.RequestRetention {
		Conf.Flashcard.RequestRetention = conf.NewFlashcard().RequestRetention
	}
	if 0 >= Conf.Flashcard.MaximumInterval || 36500 <= Conf.Flashcard.MaximumInterval {
		Conf.Flashcard.MaximumInterval = conf.NewFlashcard().MaximumInterval
	}
	if "" == Conf.Flashcard.Weights {
		Conf.Flashcard.Weights = conf.NewFlashcard().Weights
	}
	if 19 != len(strings.Split(Conf.Flashcard.Weights, ",")) {
		defaultWeights := conf.DefaultFSRSWeights()
		msg := "fsrs store weights length must be [19]"
		logging.LogWarnf("%s , given [%s], reset to default weights [%s]", msg, Conf.Flashcard.Weights, defaultWeights)
		Conf.Flashcard.Weights = defaultWeights
		scheduleLegacyConfigurationWarning(msg)
	}
	isInvalidFlashcardWeights := false
	for _, w := range strings.Split(Conf.Flashcard.Weights, ",") {
		if _, err := strconv.ParseFloat(strings.TrimSpace(w), 64); err != nil {
			isInvalidFlashcardWeights = true
			break
		}
	}
	if isInvalidFlashcardWeights {
		defaultWeights := conf.DefaultFSRSWeights()
		msg := "fsrs store weights contain invalid number"
		logging.LogWarnf("%s, given [%s], reset to default weights [%s]", msg, Conf.Flashcard.Weights, defaultWeights)
		Conf.Flashcard.Weights = defaultWeights
		scheduleLegacyConfigurationWarning(msg)
	}

	if nil == Conf.AI {
		Conf.AI = conf.NewAIWithEnvironment(util.UserAgent, os.Getenv)
	}
	if "" == Conf.AI.OpenAI.APIModel {
		Conf.AI.OpenAI.APIModel = openai.GPT3Dot5Turbo
	}
	if "" == Conf.AI.OpenAI.APIUserAgent {
		Conf.AI.OpenAI.APIUserAgent = util.UserAgent
	}
	if strings.HasPrefix(Conf.AI.OpenAI.APIUserAgent, "SiYuan/") {
		Conf.AI.OpenAI.APIUserAgent = util.UserAgent
	}
	if "" == Conf.AI.OpenAI.APIProvider {
		Conf.AI.OpenAI.APIProvider = "OpenAI"
	}
	if 0 > Conf.AI.OpenAI.APIMaxTokens {
		Conf.AI.OpenAI.APIMaxTokens = 0
	}
	if 0 >= Conf.AI.OpenAI.APITemperature || 2 < Conf.AI.OpenAI.APITemperature {
		Conf.AI.OpenAI.APITemperature = 1.0
	}
	if 1 > Conf.AI.OpenAI.APIMaxContexts || 64 < Conf.AI.OpenAI.APIMaxContexts {
		Conf.AI.OpenAI.APIMaxContexts = 7
	}

	if "" != Conf.AI.OpenAI.APIKey {
		logging.LogInfof("OpenAI API enabled")
	}

	Conf.ReadOnly = util.ReadOnly

	if "" != util.AccessAuthCode {
		Conf.AccessAuthCode = util.AccessAuthCode
	}
	Conf.AccessAuthCode = strings.TrimSpace(Conf.AccessAuthCode)
	Conf.AccessAuthCode = util.RemoveInvalid(Conf.AccessAuthCode)

	Conf.LocalIPs = util.GetLocalIPs()

	// NOTE(ToolBox): 静默处理“上次未正常完成数据索引”提示。
	// ToolBox 场景下不希望在 UI 侧弹出该提示（kernel WebSocket msg）。
	// 仍然保留 DataIndexState 的清理逻辑，避免状态一直保持为 1。

	Conf.DataIndexState = 0

	Conf.Save()
	logging.SetLogLevel(Conf.LogLevel)

	util.SetNetworkProxy(Conf.System.NetworkProxy.String())

	// Pandoc is process-once work. Its explicit owner survives workspace task
	// replacement, captures the first workspace paths and joins final shutdown.
	if err := startLegacyPandocInitialization(Conf); err != nil {
		logging.LogWarnf("start pandoc initializer failed: %s", err)
	}
}

// ReloadConf is the hot-switch-friendly version of InitConf.
// Currently it simply delegates to InitConf; the process-level Pandoc owner
// ignores repeat starts, so initialization is not restarted.
//
// Kept as a separate symbol so:
//   - Callers document intent (reload vs. initial boot)
//   - Future divergence (e.g. stricter validation during hot-switch) has
//     a natural home without touching the boot path.
//
// See docs/wiki-workspace-hotswitch/design.md §3.2.11.
func ReloadConf() {
	InitConf()
}

func initLang() {
	p := filepath.Join(util.WorkingDir, "appearance", "langs")
	dir, err := os.Open(p)
	if err != nil {
		logging.LogErrorf("open language configuration folder [%s] failed: %s", p, err)
		util.ReportFileSysFatalError(err)
		return
	}
	defer dir.Close()

	langNames, err := dir.Readdirnames(-1)
	if err != nil {
		logging.LogErrorf("list language configuration folder [%s] failed: %s", p, err)
		util.ReportFileSysFatalError(err)
		return
	}

	for _, langName := range langNames {
		jsonPath := filepath.Join(p, langName)
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			logging.LogErrorf("read language configuration [%s] failed: %s", jsonPath, err)
			continue
		}
		langMap := map[string]interface{}{}
		if err := gulu.JSON.UnmarshalJSON(data, &langMap); err != nil {
			logging.LogErrorf("parse language configuration failed [%s] failed: %s", jsonPath, err)
			continue
		}

		kernelMap := map[int]string{}
		label := langMap["_label"].(string)
		kernelLangs := langMap["_kernel"].(map[string]interface{})
		for k, v := range kernelLangs {
			num, convErr := strconv.Atoi(k)
			if nil != convErr {
				logging.LogErrorf("parse language configuration [%s] item [%d] failed: %s", p, num, convErr)
				continue
			}
			kernelMap[num] = v.(string)
		}
		kernelMap[-1] = label
		name := langName[:strings.LastIndex(langName, ".")]
		util.Langs[name] = kernelMap

		util.TimeLangs[name] = langMap["_time"].(map[string]interface{})
		util.TaskActionLangs[name] = langMap["_taskAction"].(map[string]interface{})
		util.TrayMenuLangs[name] = langMap["_trayMenu"].(map[string]interface{})
		util.AttrViewLangs[name] = langMap["_attrView"].(map[string]interface{})
	}
}

func loadLangs() (ret []*conf.Lang) {
	for name, langMap := range util.Langs {
		lang := &conf.Lang{Label: langMap[-1], Name: name}
		ret = append(ret, lang)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Name < ret[j].Name
	})
	return
}

var exitLock = sync.Mutex{}

// Close 退出内核进程.
//
// force：是否不执行同步过程而直接退出
//
// setCurrentWorkspace：是否将当前工作空间放到工作空间列表的最后一个
//
// execInstallPkg：是否执行新版本安装包
//
//	0：默认按照设置项 System.DownloadInstallPkg 检查并推送提示
//	1：不执行新版本安装
//	2：执行新版本安装
//
// 返回值 exitCode：
//
//	0：正常退出
//	1：同步执行失败
//	2：提示新安装包
//
// 当 force 为 true（强制退出）并且 execInstallPkg 为 0（默认检查更新）并且同步失败并且新版本安装版已经准备就绪时，执行新版本安装 https://github.com/siyuan-note/siyuan/issues/10288
func Close(force, setCurrentWorkspace bool, execInstallPkg int) (exitCode int) {
	return CloseContext(context.Background(), force, setCurrentWorkspace, execInstallPkg)
}

// Preserve the initiating HTTP request identity through service teardown. The
// transport drains every other handler before this control response is written.
func CloseContext(ctx context.Context, force, setCurrentWorkspace bool, execInstallPkg int) (exitCode int) {
	exitLock.Lock()
	defer exitLock.Unlock()

	logging.LogInfof("exiting kernel [force=%v, setCurrentWorkspace=%v, execInstallPkg=%d]", force, setCurrentWorkspace, execInstallPkg)
	util.PushMsg(Conf.Language(95), 10000*60)
	FlushTxQueue()

	if !force {
		if Conf.Sync.Enabled && 3 != Conf.Sync.Mode &&
			((IsSubscriber() && conf.ProviderSiYuan == Conf.Sync.Provider) || conf.ProviderSiYuan != Conf.Sync.Provider) {
			syncData(true, false)
			if 0 != ExitSyncStatus() {
				exitCode = 1
				return
			}
		}
	}

	// Close the user guide when exiting https://github.com/siyuan-note/siyuan/issues/10322
	closeUserGuide()

	// Improve indexing completeness when exiting https://github.com/siyuan-note/siyuan/issues/12039
	sql.FlushQueue()

	waitSecondForExecInstallPkg := false
	if !skipNewVerInstallPkg() && "" != newVerInstallPkgPath {
		if 2 == execInstallPkg || (force && 0 == execInstallPkg) { // 执行新版本安装
			waitSecondForExecInstallPkg = true
			if gulu.OS.IsWindows() {
				util.PushMsg(Conf.Language(130), 1000*30)
			}
		} else if 0 == execInstallPkg { // 新版本安装包已经准备就绪
			exitCode = 2
			logging.LogInfof("the new version install pkg is ready [%s], waiting for the user's next instruction", newVerInstallPkgPath)
			return
		}
	}

	// Only stop producers after all exit-veto decisions. Actual callbacks must
	// return before caches/DBs close; a UI timeout is not a completed handler.
	// Restore writes/rebuilds need the old SQL consumers alive. Close their
	// admission before stopping watchers, otherwise a finishing restore could
	// reopen a watcher in the gap before the generic task owner is stopped.
	if err := Conf.StopRepoCheckouts(context.Background()); err != nil {
		return 1
	}
	if err := Conf.CloseCloudRepo(context.Background()); err != nil {
		return 1
	}
	if cronCtrl.Stop != nil {
		cronCtrl.Stop()
	}
	StopAutoGenerateFileHistory()
	if cronCtrl.Drain != nil {
		cronCtrl.Drain()
	}
	for _, extra := range cronCtrl.DrainExtra {
		extra()
	}
	DrainAutoGenerateFileHistory()
	StopWatchAssets()
	StopWatchEmojis()
	CloseWatchAssets()
	CloseWatchEmojis()
	CloseThemeWatchers()
	// Stop listeners and active request owners while transaction queues and DBs
	// are still available to requests that were already admitted.
	if err := StopLegacyServices(ctx); err != nil {
		logging.LogErrorf("close runtime services failed: %s", err)
		return 1
	}
	if err := closeLegacyBootTasks(context.Background()); err != nil {
		logging.LogErrorf("close boot tasks failed: %s", err)
		return 1
	}
	if err := task.CloseLegacyTasks(context.Background()); err != nil {
		logging.LogErrorf("close task handlers failed: %s", err)
		return 1
	}
	// Stop transaction admission and join its explicit owner before closing DBs.
	// Earlier sync/update exits leave the queue live so the GUI can keep editing.
	if err := CloseTransactionQueue(context.Background()); err != nil {
		logging.LogErrorf("close transaction queue failed: %s", err)
		return 1
	}
	// Transactions admitted during the earlier sync/update checks may have added
	// index work. Flush it after the worker joins, before IsExiting stops SQL jobs.
	sql.FlushQueue()
	sql.FlushHistoryQueue()
	sql.FlushAssetContentQueue()
	util.IsExiting.Store(true)
	if waitSecondForExecInstallPkg {
		go execNewVerInstallPkg(newVerInstallPkgPath)
	}
	Conf.Close()
	if err := StopLegacySubscriptions(context.Background()); err != nil {
		logging.LogErrorf("close event subscriptions failed: %s", err)
		return 1
	}
	sql.CloseDatabase()
	if err := CloseLegacyRuntime(ctx); err != nil {
		logging.LogErrorf("close legacy runtime failed: %s", err)
		return 1
	}
	util.SaveAssetsTexts()
	clearWorkspaceTemp()
	clearCorruptedNotebooks()
	clearPortJSON()

	if setCurrentWorkspace {
		// 将当前工作空间放到工作空间列表的最后一个
		// Open the last workspace by default https://github.com/siyuan-note/siyuan/issues/10570
		workspacePaths, err := util.ReadWorkspacePaths()
		if err != nil {
			logging.LogErrorf("read workspace paths failed: %s", err)
		} else {
			workspacePaths = gulu.Str.RemoveElem(workspacePaths, util.WorkspaceDir)
			workspacePaths = append(workspacePaths, util.WorkspaceDir)
			util.WriteWorkspacePaths(workspacePaths)
		}
	}

	util.UnlockWorkspace()

	time.Sleep(500 * time.Millisecond)
	if waitSecondForExecInstallPkg {
		// 桌面端退出拉起更新安装时有时需要重启两次 https://github.com/siyuan-note/siyuan/issues/6544
		// 这里多等待一段时间，等待安装程序启动
		if gulu.OS.IsWindows() {
			time.Sleep(30 * time.Second)
		}
	}
	closeSyncWebSocket()
	go func() {
		time.Sleep(500 * time.Millisecond)
		logging.LogInfof("exited kernel")
		util.HttpServing = false
		os.Exit(logging.ExitCodeOk)
	}()
	return
}

func AddCustomEmoji(name, src string) {
	if Conf != nil {
		Conf.customEmojis.Put(name, src)
	}
}

func ClearCustomEmojis() {
	if Conf != nil {
		Conf.customEmojis.Clear()
	}
}

func originalEditorOptions() (nativeparser.EditorOptions, error) {
	return originalEditorOptionsContext(context.Background())
}
func originalMarkdownOptionsContext(ctx context.Context) (nativeparser.Markdown, error) {
	if ctx == nil {
		return nativeparser.Markdown{}, conf.ErrBusinessUnbound
	}
	if e := ctx.Err(); e != nil {
		return nativeparser.Markdown{}, e
	}
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nativeparser.Markdown{}, e
	}
	owner := base.owner
	owner.m.Lock()
	if owner.Editor == nil || owner.Editor.Markdown == nil {
		owner.m.Unlock()
		return nativeparser.Markdown{}, conf.ErrBusinessUnbound
	}
	options := *owner.Editor.Markdown
	owner.m.Unlock()
	if owner != Conf {
		return nativeparser.Markdown{}, conf.ErrBusinessUnbound
	}
	return options, ctx.Err()
}

func originalEditorOptionsContext(ctx context.Context) (nativeparser.EditorOptions, error) {
	if ctx == nil {
		return nativeparser.EditorOptions{}, conf.ErrBusinessUnbound
	}
	if err := ctx.Err(); err != nil {
		return nativeparser.EditorOptions{}, err
	}
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nativeparser.EditorOptions{}, e
	}
	owner := base.owner
	customEmojiMap := owner.customEmojis.Snapshot()
	owner.m.Lock()
	options, err := owner.AppData.ParserOptions(customEmojiMap)
	owner.m.Unlock()
	if err != nil {
		return nativeparser.EditorOptions{}, err
	}
	if owner != Conf {
		return nativeparser.EditorOptions{}, conf.ErrBusinessUnbound
	}
	return options, ctx.Err()
}

func NewLuteContext(ctx context.Context) (*lute.Lute, error) {
	options, err := originalEditorOptionsContext(ctx)
	if err != nil {
		return nil, err
	}
	return nativeparser.NewEditor(options), nil
}

func NewLute() (ret *lute.Lute) {
	options, err := originalEditorOptions()
	if err != nil {
		panic(err)
	} // original model requires initialized configuration
	ret = nativeparser.NewEditor(options)
	return
}

func enableLuteInlineSyntax(luteEngine *lute.Lute) {
	luteEngine.SetInlineAsterisk(true)
	luteEngine.SetInlineUnderscore(true)
	luteEngine.SetSup(true)
	luteEngine.SetSub(true)
	luteEngine.SetTag(true)
	luteEngine.SetInlineMath(true)
	luteEngine.SetGFMStrikethrough(true)
}

func (conf *AppConf) Save() {
	if err := conf.SaveError(); err != nil {
		logging.LogErrorf("write conf [%s] failed: %s", conf.confPath, err)
		if !errors.Is(err, ErrUnboundAppConf) {
			util.ReportFileSysFatalError(err)
		}
	}
}

// SaveError persists this instance only. JSON ReadOnly is presentation data,
// not write authority; imported or masking-only configurations stay unbound.
func (conf *AppConf) SaveError() error {
	conf.m.Lock()
	defer conf.m.Unlock()
	return conf.saveDataLocked(conf.AppData)
}

func (conf *AppConf) Close() {
	conf.Save()
}

func (conf *AppConf) Box(boxID string) *Box {
	for _, box := range conf.GetOpenedBoxes() {
		if box.ID == boxID {
			return box
		}
	}
	return nil
}

func (conf *AppConf) GetBox(boxID string) *Box {
	for _, box := range conf.GetBoxes() {
		if box.ID == boxID {
			return box
		}
	}
	return nil
}

func (conf *AppConf) BoxNames(boxIDs []string) (ret map[string]string) {
	ret = map[string]string{}

	boxes := conf.GetOpenedBoxes()
	for _, boxID := range boxIDs {
		for _, box := range boxes {
			if box.ID == boxID {
				ret[boxID] = box.Name
				break
			}
		}
	}
	return
}

func (conf *AppConf) GetBoxes() (ret []*Box) {
	ret = []*Box{}
	notebooks, err := conf.notebooks(context.Background())
	if err != nil {
		return
	}

	for _, notebook := range notebooks {
		id := notebook.ID
		name := notebook.Name
		closed := notebook.Closed
		box := &Box{ID: id, Name: name, Closed: closed}
		ret = append(ret, box)
	}
	return
}

func (conf *AppConf) GetOpenedBoxes() (ret []*Box) {
	ret = []*Box{}
	notebooks, err := conf.notebooks(context.Background())
	if err != nil {
		return
	}

	for _, notebook := range notebooks {
		if !notebook.Closed {
			ret = append(ret, notebook)
		}
	}
	return
}

func (conf *AppConf) GetClosedBoxes() (ret []*Box) {
	ret = []*Box{}
	notebooks, err := conf.notebooks(context.Background())
	if err != nil {
		return
	}

	for _, notebook := range notebooks {
		if notebook.Closed {
			ret = append(ret, notebook)
		}
	}
	return
}

func (conf *AppConf) Language(num int) (ret string) {
	ret = conf.business.Language(conf.Lang, num, true, conf.CloudRegion)
	return
}

func (conf *AppConf) language(num int) (ret string) {
	ret = conf.business.Language(conf.Lang, num, false, conf.CloudRegion)
	return
}

func InitBoxes() {
	if e := InitBoxesContext(context.Background()); e != nil {
		logging.LogErrorf("initialize notebooks failed: %s", e)
	}
}

func IsSubscriber() bool {
	u := Conf.GetUser()
	return nil != u && (-1 == u.UserSiYuanProExpireTime || 0 < u.UserSiYuanProExpireTime) && 0 == u.UserSiYuanSubscriptionStatus
}

func IsPaidUser() bool {
	// S3/WebDAV data sync and backup are available for a fee https://github.com/siyuan-note/siyuan/issues/8780

	if IsSubscriber() {
		return true
	}

	u := Conf.GetUser()
	if nil == u {
		return false
	}
	return 1 == u.UserSiYuanOneTimePayStatus
}

const (
	MaskedUserData       = ""
	MaskedAccessAuthCode = "*******"
)

func GetMaskedConf() (*AppConf, error) {
	return GetMaskedConfContext(context.Background())
}

func GetMaskedConfContext(ctx context.Context) (*AppConf, error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	value, e := o.owner.Masked()
	if e != nil {
		return nil, e
	}
	if e = o.guard(ctx); e != nil {
		return nil, e
	}
	return value, nil
}

func (c *AppConf) Masked() (*AppConf, error) {
	c.m.Lock()
	defer c.m.Unlock()
	data, e := c.AppData.Masked()
	if e != nil {
		return nil, e
	}
	return &AppConf{AppData: data}, nil
}

// REF: https://github.com/siyuan-note/siyuan/issues/11364
// HideConfSecret 隐藏设置中的秘密信息
func HideConfSecret(c *AppConf) { c.AppData.HideSecrets() }

func clearPortJSON() {
	pid := fmt.Sprintf("%d", os.Getpid())
	portJSON := filepath.Join(util.HomeDir, ".config", "siyuan", "port.json")
	pidPorts := map[string]string{}
	var data []byte
	var err error

	if gulu.File.IsExist(portJSON) {
		data, err = os.ReadFile(portJSON)
		if err != nil {
			logging.LogWarnf("read port.json failed: %s", err)
		} else {
			if err = gulu.JSON.UnmarshalJSON(data, &pidPorts); err != nil {
				logging.LogWarnf("unmarshal port.json failed: %s", err)
			}
		}
	}

	delete(pidPorts, pid)
	if data, err = gulu.JSON.MarshalIndentJSON(pidPorts, "", "  "); err != nil {
		logging.LogWarnf("marshal port.json failed: %s", err)
	} else {
		if err = os.WriteFile(portJSON, data, 0644); err != nil {
			logging.LogWarnf("write port.json failed: %s", err)
		}
	}
}

func clearCorruptedNotebooks() {
	// 数据同步时展开文档树操作可能导致数据丢失 https://github.com/siyuan-note/siyuan/issues/7129

	dirs, err := os.ReadDir(util.DataDir)
	if err != nil {
		logging.LogErrorf("read dir [%s] failed: %s", util.DataDir, err)
		return
	}
	for _, dir := range dirs {
		if util.IsReservedFilename(dir.Name()) {
			continue
		}

		if !dir.IsDir() {
			continue
		}

		if !ast.IsNodeIDPattern(dir.Name()) {
			continue
		}

		boxDirPath := filepath.Join(util.DataDir, dir.Name())
		boxConfPath := filepath.Join(boxDirPath, ".siyuan", "conf.json")
		if !filelock.IsExist(boxConfPath) {
			logging.LogWarnf("found a corrupted box [%s]", boxDirPath)
			continue
		}
	}
}

func clearWorkspaceTemp() {
	os.RemoveAll(filepath.Join(util.TempDir, "bazaar"))
	os.RemoveAll(filepath.Join(util.TempDir, "export"))
	os.RemoveAll(filepath.Join(util.TempDir, "convert"))
	os.RemoveAll(filepath.Join(util.TempDir, "import"))
	os.RemoveAll(filepath.Join(util.TempDir, "repo"))
	os.RemoveAll(filepath.Join(util.TempDir, "os"))
	os.RemoveAll(filepath.Join(util.TempDir, "base64"))
	os.RemoveAll(filepath.Join(util.TempDir, "blocktree.msgpack")) // v2.7.2 前旧版的块树数据
	os.RemoveAll(filepath.Join(util.TempDir, "blocktree"))         // v3.1.0 前旧版的块树数据

	// 退出时自动删除超过 7 天的安装包 https://github.com/siyuan-note/siyuan/issues/6128
	install := filepath.Join(util.TempDir, "install")
	if gulu.File.IsDir(install) {
		monthAgo := time.Now().Add(-time.Hour * 24 * 7)
		entries, err := os.ReadDir(install)
		if err != nil {
			logging.LogErrorf("read dir [%s] failed: %s", install, err)
		} else {
			for _, entry := range entries {
				info, _ := entry.Info()
				if nil != info && !info.IsDir() && info.ModTime().Before(monthAgo) {
					if err = os.RemoveAll(filepath.Join(install, entry.Name())); err != nil {
						logging.LogErrorf("remove old install pkg [%s] failed: %s", filepath.Join(install, entry.Name()), err)
					}
				}
			}
		}
	}

	tmps, err := filepath.Glob(filepath.Join(util.TempDir, "*.tmp"))
	if err != nil {
		logging.LogErrorf("glob temp files failed: %s", err)
	}
	for _, tmp := range tmps {
		if err = os.RemoveAll(tmp); err != nil {
			logging.LogErrorf("remove temp file [%s] failed: %s", tmp, err)
		} else {
			logging.LogInfof("removed temp file [%s]", tmp)
		}
	}

	tmps, err = filepath.Glob(filepath.Join(util.DataDir, ".siyuan", "*.tmp"))
	if err != nil {
		logging.LogErrorf("glob temp files failed: %s", err)
	}
	for _, tmp := range tmps {
		if err = os.RemoveAll(tmp); err != nil {
			logging.LogErrorf("remove temp file [%s] failed: %s", tmp, err)
		} else {
			logging.LogInfof("removed temp file [%s]", tmp)
		}
	}

	// 老版本遗留文件清理
	os.RemoveAll(filepath.Join(util.DataDir, "assets", ".siyuan", "assets.json"))
	os.RemoveAll(filepath.Join(util.DataDir, ".siyuan", "history"))
	os.RemoveAll(filepath.Join(util.WorkspaceDir, "backup"))
	os.RemoveAll(filepath.Join(util.WorkspaceDir, "sync"))
	os.RemoveAll(filepath.Join(util.DataDir, "%")) // v3.0.6 生成的错误历史文件夹

	logging.LogInfof("cleared workspace temp")
}

func closeUserGuide() {
	defer logging.Recover()

	dirs, err := os.ReadDir(util.DataDir)
	if err != nil {
		logging.LogErrorf("read dir [%s] failed: %s", util.DataDir, err)
		return
	}

	for _, dir := range dirs {
		if !IsUserGuide(dir.Name()) {
			continue
		}

		boxID := dir.Name()
		boxDirPath := filepath.Join(util.DataDir, boxID)
		boxConf := conf.NewBoxConf()
		boxConfPath := filepath.Join(boxDirPath, ".siyuan", "conf.json")
		if !filelock.IsExist(boxConfPath) {
			logging.LogWarnf("found a corrupted user guide box [%s]", boxDirPath)
			if removeErr := filelock.Remove(boxDirPath); nil != removeErr {
				logging.LogErrorf("remove corrupted user guide box [%s] failed: %s", boxDirPath, removeErr)
			} else {
				logging.LogInfof("removed corrupted user guide box [%s]", boxDirPath)
			}
			continue
		}

		data, readErr := filelock.ReadFile(boxConfPath)
		if nil != readErr {
			logging.LogErrorf("read box conf [%s] failed: %s", boxConfPath, readErr)
			if removeErr := filelock.Remove(boxDirPath); nil != removeErr {
				logging.LogErrorf("remove corrupted user guide box [%s] failed: %s", boxDirPath, removeErr)
			} else {
				logging.LogInfof("removed corrupted user guide box [%s]", boxDirPath)
			}
			continue
		}
		if readErr = gulu.JSON.UnmarshalJSON(data, boxConf); nil != readErr {
			logging.LogErrorf("parse box conf [%s] failed: %s", boxConfPath, readErr)
			if removeErr := filelock.Remove(boxDirPath); nil != removeErr {
				logging.LogErrorf("remove corrupted user guide box [%s] failed: %s", boxDirPath, removeErr)
			} else {
				logging.LogInfof("removed corrupted user guide box [%s]", boxDirPath)
			}
			continue
		}

		if boxConf.Closed {
			continue
		}

		msgId := util.PushMsg(Conf.language(233), 30000)
		evt := util.NewCmdResult("unmount", 0, util.PushModeBroadcast)
		evt.Data = map[string]interface{}{
			"box": boxID,
		}
		util.PushEvent(evt)

		unindex(boxID)

		sql.FlushQueue()
		if removeErr := filelock.Remove(boxDirPath); nil != removeErr {
			logging.LogErrorf("remove corrupted user guide box [%s] failed: %s", boxDirPath, removeErr)
		}

		util.PushClearMsg(msgId)
		logging.LogInfof("closed user guide box [%s]", boxID)
	}
}

// Presentation preserves original UI fields using one owned configuration snapshot.
func (c *AppConf) Presentation(restricted, publish bool) (*AppConf, error) {
	c.m.Lock()
	defer c.m.Unlock()
	if c.business == nil {
		return nil, conf.ErrBusinessUnbound
	}
	data, e := c.AppData.Presentation(conf.PresentationOptions{Restricted: restricted, Publish: publish, DisabledSyncText: c.business.Language(c.Lang, 53, true, c.CloudRegion)})
	if e != nil {
		return nil, e
	}
	return &AppConf{AppData: data}, nil
}
