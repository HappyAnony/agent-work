import type {App} from "../index";
import type {Tab} from "../layout/Tab";
import {Files} from "../layout/dock/Files";
import {Bookmark} from "../layout/dock/Bookmark";
import {Tag} from "../layout/dock/Tag";
import {Outline} from "../layout/dock/Outline";
import {Backlink} from "../layout/dock/Backlink";
import {SingleDocHeaderMeta} from "../single/SingleDocHeaderMeta";
import {SingleOutline} from "../single/SingleOutline";
import {ensureSingleLayoutStyleElement} from "../single/layoutStyle";
import {fetchSyncPost} from "../util/fetch";
import {Constants} from "../constants";
import {setReadonlyByConfig} from "../protyle/util/onGet";
import {showMessage} from "../dialog/message";
import type {ProtyleBinding} from "./protyle";
import {installScopedProviders} from "./providers";
import {EditorSession, SessionError} from "./session";
import {matchHotKey} from "../protyle/util/hotKey";
import {portableKeymapPreferences} from "./keymap";
import {mountScopedTabs} from "./tabs";

export function preparePortableLayout(element: HTMLElement, single: boolean) {
    const workspace = document.createElement("div");
    const sidebar = document.createElement("aside");
    const pane = document.createElement("main");
    if (single) {
        workspace.id = "singleWorkspace";
        sidebar.id = "singleOutline";
        pane.id = "singleEditorPane";
        ensureSingleLayoutStyleElement();
    } else {
        workspace.className = "fn__flex portable-desktop";
        workspace.style.height = "100vh";
        sidebar.className = "fn__flex-column";
        sidebar.style.cssText = "width:260px;min-width:180px;min-height:0;flex-shrink:0;overflow:auto;border-right:1px solid var(--b3-border-color)";
        pane.className = "fn__flex-column fn__flex-1";
        pane.style.cssText = "height:100%;min-height:0;min-width:0;overflow:hidden";
    }
    element.before(workspace);
    element.style.cssText = single ? "height:100%;min-height:0" : "flex:1;min-height:0;overflow:hidden";
    workspace.append(sidebar, pane);
    pane.append(element);
    return {workspace, sidebar, pane};
}

async function mountFiles(sidebar: HTMLElement, app: App, openDocument: (id: string, event: MouseEvent) => void,
                          currentDocument?: () => {box: string; path: string}, binding?: ProtyleBinding, protyle?: IProtyle, renameTag?: (label: string) => void) {
    const notebooks = await fetchSyncPost("/api/notebook/lsNotebooks", {});
    if (notebooks.code !== 0) {
        throw new SessionError(notebooks.msg);
    }
    window.siyuan.notebooks = notebooks.data.notebooks;
    const toggleFiles = document.createElement("button");
    toggleFiles.type = "button";
    toggleFiles.className = "dock__item dock__item--active";
    toggleFiles.dataset.type = "file";
    toggleFiles.setAttribute("aria-label", window.siyuan.languages.fileTree);
    toggleFiles.setAttribute("aria-expanded", "true");
    toggleFiles.innerHTML = '<svg><use xlink:href="#iconFiles"></use></svg>';
    const dock = document.createElement("div");
    dock.className = "dock dock--vertical";
    dock.append(toggleFiles);
    const addDockAction = (type: string, label: string, icon: string, action: () => void) => {
        const button = document.createElement("button");
        button.type = "button";
        button.dataset.type = type;
        button.className = "dock__item";
        button.setAttribute("aria-label", label);
        button.innerHTML = `<svg><use xlink:href="#${icon}"></use></svg>`;
        button.addEventListener("click", action);
        dock.append(button);
    };
    const openSearch = (keyword?: string) => {
        void import("../search/scoped").then(({openScopedSearch}) => {
            if (sidebar.isConnected) { openScopedSearch(binding, keyword); }
        }).catch(() => {
            if (sidebar.isConnected) { showMessage(window.siyuan.languages.operationFailed ?? "Search unavailable"); }
        });
    };
    const searchShortcut = (event: KeyboardEvent) => {
        if (!event.isComposing && matchHotKey(portableKeymapPreferences().general.globalSearch.custom, event)) {
            event.preventDefault();
            event.stopPropagation();
            openSearch();
        }
    };
    let visiblePanel: HTMLElement | null = sidebar;
    let visibleToggle: HTMLButtonElement | null = toggleFiles;
    const switchPanel = (panel: HTMLElement, button: HTMLButtonElement) => {
        const show = visiblePanel !== panel;
        visiblePanel?.classList.add("fn__none");
        visibleToggle?.classList.remove("dock__item--active");
        visibleToggle?.setAttribute("aria-expanded", "false");
        visiblePanel = show ? panel : null;
        visibleToggle = show ? button : null;
        if (show) {
            panel.classList.remove("fn__none");
            button.classList.add("dock__item--active");
            button.setAttribute("aria-expanded", "true");
        }
    };
    const panels: Array<() => void> = [];
    const kinds: Array<"bookmark" | "tag" | "outline" | "backlink"> = ["bookmark", "tag"];
    if (binding && protyle) { kinds.unshift("outline","backlink"); }
    for (const kind of kinds) {
        const panel = document.createElement("aside");
        panel.style.cssText = sidebar.style.cssText;
        panel.classList.add("fn__none");
        sidebar.after(panel);
        let model: Bookmark | Tag | Outline | Backlink;
        const icon = {bookmark:"iconBookmark",tag:"iconTags",outline:"iconAlignCenter",backlink:"iconLink"}[kind];
        addDockAction(kind, window.siyuan.languages[kind==="backlink"?"backlinks":kind], icon, () => {
            switchPanel(panel, button);
            if (!model) {
                const tab = {id: crypto.randomUUID(), panelElement: panel} as Tab;
                const minimize = () => { if (visiblePanel === panel) { switchPanel(panel, button); button.focus(); } };
                if (kind === "bookmark") { model = new Bookmark(app, tab, minimize); }
                else if (kind === "tag") { model = new Tag(app, tab, {minimize, rename:renameTag, search:label => openSearch("#" + label + "#")}); }
                else if (kind === "backlink") {
                    model = new Backlink({app,tab,type:"pin",blockId:binding.document.docId,rootId:binding.document.docId,minimize});
                }
                else {
                    model = new Outline({app, tab, type:"pin", blockId:binding.document.docId, isPreview:false,
                        scoped:{minimize, editor:protyle, focus:id => {
                            if (id === binding.document.docId) { protyle.contentElement.scrollTop = 0; return; }
                            try { binding.focusReference(id); } catch (error) { showMessage(error.message); }
                        }},
                    });
                }
            }
        });
        const button = dock.lastElementChild as HTMLButtonElement;
        button.setAttribute("aria-expanded", "false");
        panels.push(() => model?.send("closews", {}));
    }
    addDockAction("search", window.siyuan.languages.globalSearch, "iconSearch", () => openSearch());
    addDockAction("globalGraph", window.siyuan.languages.globalGraph, "iconGlobalGraph", () => {
        void import("../layout/dock/Graph").then(({openScopedGraph}) => {
            if (sidebar.isConnected) { openScopedGraph(app); }
        }).catch(() => {
            if (sidebar.isConnected) { showMessage(window.siyuan.languages.operationFailed ?? "Graph unavailable"); }
        });
    });
    document.addEventListener("keydown", searchShortcut, true);
    sidebar.before(dock);
    toggleFiles.addEventListener("click", () => switchPanel(sidebar, toggleFiles));
    const files = new Files({
        app, tab: {id: "portable-files", panelElement: sidebar} as Tab, openDocument, currentDocument,
        minimize: () => {
            if (visiblePanel === sidebar) { switchPanel(sidebar, toggleFiles); }
            toggleFiles.focus();
        },
    });
    const settings = document.createElement("button");
    settings.type = "button";
    settings.dataset.type = "portable-settings";
    settings.className = "block__icon b3-tooltips b3-tooltips__sw";
    settings.setAttribute("aria-label", window.siyuan.languages.config);
    settings.innerHTML = '<svg><use xlink:href="#iconSettings"></use></svg>';
    settings.addEventListener("click", () => {
        void import("../config/portableSettings").then(({openPortableSettings}) => {
            if (sidebar.isConnected) { openPortableSettings(app); }
        }).catch(() => {
            if (sidebar.isConnected) { showMessage(window.siyuan.languages.operationFailed ?? "Settings unavailable"); }
        });
    });
    const actions = sidebar.querySelector('.block__icons [data-type="more"]');
    actions.before(settings);
    const history = document.createElement("button");
    history.type = "button";
    history.dataset.type = "portable-history";
    history.className = settings.className;
    history.setAttribute("aria-label", window.siyuan.languages.dataHistory);
    history.innerHTML = '<svg><use xlink:href="#iconHistory"></use></svg>';
    history.addEventListener("click", () => {
        void import("../history/history").then(({openHistory}) => {
            if (sidebar.isConnected) { openHistory(app); }
        }).catch(() => {
            if (sidebar.isConnected) { showMessage(window.siyuan.languages.operationFailed ?? "History unavailable"); }
        });
    });
    actions.before(history);
    return {close() {
        document.removeEventListener("keydown", searchShortcut, true);
        panels.forEach(close => close());
        files.send("closews", {});
    }, selectItem: files.selectItem.bind(files)};
}

export async function mountPortableLayout(sidebar: HTMLElement, binding: ProtyleBinding, protyle: IProtyle, app: App,
                                          notice: (message: string) => void, title?: (value: string) => void, renameTag?: (label: string) => void) {
    const id = binding.document.docId;
    let headerGeneration = 0;
    const refreshHeader = async () => {
        const generation = ++headerGeneration;
        const response = await fetchSyncPost("/api/block/getDocInfo", {id});
        if (generation === headerGeneration && response.code === 0 && response.data?.ial && protyle.element.isConnected) {
            title?.(response.data.ial.title);
            if (document.activeElement !== protyle.title.editElement) {
                protyle.title.element.removeAttribute("data-render");
                protyle.title.render(protyle, {...response, data: {attrViews: [], ...response.data}});
            }
            const content = protyle.wysiwyg.element;
            const readonly = window.siyuan.config.readonly || window.siyuan.config.editor.readOnly ||
                response.data.ial[Constants.CUSTOM_SY_READONLY] === "true";
            if (response.data.ial[Constants.CUSTOM_SY_READONLY] === "true") {
                content.setAttribute(Constants.CUSTOM_SY_READONLY, "true");
            } else {
                content.removeAttribute(Constants.CUSTOM_SY_READONLY);
            }
            if (content.getAttribute("data-readonly") !== String(readonly)) {
                setReadonlyByConfig(protyle, true);
            }
            protyle.breadcrumb?.render(protyle);
        }
    };
    await refreshHeader();
    binding.onDetach(() => { headerGeneration++; });
    binding.onDetach(binding.session.onWorkspaceEvent(event => {
        const effect = event.effect as {documents?: string[]} | undefined;
        if (event.type === "workspaceEvent" && effect?.documents?.includes(id)) {
            void refreshHeader().catch(error => notice(error.message));
        }
    }));
    if (binding.session.view.mode === "docxSingle") {
        const meta = new SingleDocHeaderMeta({blockId: id, protyle});
        const outline = new SingleOutline({blockId: id, protyle});
        binding.onDetach(() => {
            meta.destroy();
            outline.destroy();
        });
        binding.onDetach(binding.session.onWorkspaceEvent(event => {
            if (event.type === "workspaceEvent") {
                meta.refreshSoon();
                outline.refreshSoon();
            }
        }));
        return;
    }
    const files = await mountFiles(sidebar, app, (target,event) => {
        const reuse=window.siyuan.config.fileTree.openFilesUseCurrentTab&&!event.ctrlKey&&!event.metaKey&&!event.altKey;
        void binding.openDocument(target,reuse).catch(error => notice(error.message));
    }, () => ({box: binding.document.boxId, path: protyle.path}), binding, protyle, renameTag);
    binding.onDetach(files.close);
    await files.selectItem(binding.document.boxId, protyle.path, undefined, false);
}

export async function mountEmptyPortableDesktop(element: HTMLElement, session: EditorSession, app: App) {
    const {workspace, sidebar, pane} = preparePortableLayout(element, false);
    const notice = document.createElement("p");
    notice.setAttribute("role", "status");
    notice.textContent = window.siyuan.languages.emptyContent;
    element.append(notice);
    let navigate: (id: string) => Promise<void>;
    const pending = new Set<Promise<unknown>>();
    const cleanup = [installScopedProviders({
        post(endpoint, data) {
            const request = session.requestUI(endpoint, data);
            pending.add(request);
            return request.finally(() => pending.delete(request));
        },
        get() { return Promise.reject(new SessionError("ui_resource_unavailable")); },
        error(error) { notice.textContent = error instanceof Error ? error.message : "ui_request_failed"; },
        subscribe: listener => session.onWorkspaceEvent(listener),
        openDocument(id) {
            return navigate ? navigate(id) : Promise.reject(new SessionError("reference_navigation_unavailable"));
        },
    })];
    const tabs = mountScopedTabs(workspace, pane, app, message => { notice.textContent = message; });
    cleanup.push(tabs.destroy);
    const openDocument = (id: string) => {
        if (navigate && session.status === "active") {
            void navigate(id).catch(error => { notice.textContent = error.message; });
        }
    };
    const detach = () => {
        cleanup.splice(0).forEach(dispose => dispose());
        session.close();
        workspace.remove();
    };
    try {
        await session.subscribe(() => undefined);
        const files = await mountFiles(sidebar, app, openDocument);
        cleanup.push(files.close);
    } catch (error) {
        detach();
        throw error;
    }
    return {
        tabs,
        detach,
        onDetach(dispose: () => void) { cleanup.push(dispose); },
        setReferenceNavigator(handler: (id: string) => Promise<void>) { navigate = handler; },
        async prepareRelease() {
            if (pending.size) { throw new SessionError("unsaved_editor_state"); }
            await session.prepareRelease();
        },
        focusReference() { throw new SessionError("reference_focus_unavailable"); },
        handleFocusedHistory() { return false; },
        async prepareTabSwitch() {
            if (pending.size) { throw new SessionError("unsaved_editor_state"); }
            await session.flush();
        },
        focusTab() {},
        get canRestoreHistory() { return false; },
    };
}
