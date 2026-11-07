import {installScopedBlockPopover} from "../block/popover";
import {Protyle} from "../protyle";
import {Constants} from "../constants";
import {installScopedProviders} from "./providers";
import {setEditMode} from "../protyle/util/setEditMode";
import {contentMenu} from "../menus/protyle";
import {openAttr} from "../menus/commonMenuItem";
import {isOnlyMeta} from "../protyle/util/compatibility";
import {DOC_VERSIONS_ENDPOINT, isArchiveEndpoint, EditorSession, DocumentKey, SessionError} from "./session";
import {ProtyleBinding} from "./protyle";
import {genAssetHTML} from "../asset/renderAssets";
import {installPortableMenu} from "./menu";
import type {ProtyleOperation} from "./session";
import {PORTABLE_FORMAT_TOOLS} from "./tools";
import {applyPortableAppearance, UIConfiguration} from "./ui";
import {activePortableKeymap, loadPortableKeymap} from "./keymap";
import {setCodeTheme} from "../util/assets";
import {uploadFiles as uploadProtyleFiles} from "../protyle/upload";
import {mountPortableLayout, preparePortableLayout} from "./layout";
import {mountScopedTabs} from "./tabs";
import {codeBlockOptions} from "../protyle/gutter/codeOptions";
import {Menu} from "../plugin/Menu";
import {MenuItem} from "../menus/Menu";
import {Dialog} from "../dialog";
import {renameTag} from "../util/noRelyPCFunction";
import {fetchPost} from "../util/fetch";
import type {FrameTabs} from "./bridge";
import {showExternalPreview} from "./externalPreview";
import {mountScopedPlugins} from "./scopedPlugins";
import type {ExternalKind} from "./external";
import "../assets/scss/base.scss";
import "../assets/scss/portable.scss";

/** Actual rich-text factory for a dedicated, initialized wiki-app window.
 * Source-independent UI preferences/languages/Lute/CSS must be initialized by
 * its controlled boot entry, never by querying legacy getConf for a workspace.
 * The controlled boot initializes these defaults without a legacy workspace. */
export async function mountPortableProtyle(options:{
    element:HTMLElement;session:EditorSession;document:DocumentKey;
    hostPluginLayout?: boolean;
    app:ConstructorParameters<typeof Protyle>[0];
}):Promise<{protyle:Protyle;binding:ProtyleBinding & {tabs?: FrameTabs};dispose():Promise<void>}> {
    if(!window.siyuan?.config?.editor || !window.siyuan.languages || typeof Lute==="undefined") {throw new SessionError("portable_ui_bootstrap_required");}
    if(options.app.plugins.length!==0){throw new SessionError("portable_plugins_unsupported");}
    const binding: ProtyleBinding & {tabs?: FrameTabs} = new ProtyleBinding(options.session,options.document);
    const single = options.session.view.mode === "docxSingle";
    if (!single) {
        const [ai, widget] = await Promise.all(["/api/ai/status", "/api/widget/status"].map(endpoint =>
            options.session.requestUI(endpoint).catch(() => false)));
        binding.aiWritingEnabled = ai === true;
        binding.widgetEnabled = widget === true;
    }
    const layout = preparePortableLayout(options.element, single);
    const toolbar=document.createElement("div"); toolbar.className="portable-document-actions"; toolbar.setAttribute("role","toolbar"); toolbar.setAttribute("aria-label","Wiki document");
    const languages=window.siyuan.languages;
    const upload=document.createElement("button"); upload.textContent=languages.upload; upload.type="button";
    const picker=document.createElement("input"); picker.type="file"; picker.multiple=true; picker.hidden=true; picker.setAttribute("aria-label",languages.upload);
    const exportButton=document.createElement("button"); exportButton.textContent=languages.export+" .docj"; exportButton.type="button";
    const htmlButton=document.createElement("button");htmlButton.textContent=languages.export+" HTML";htmlButton.type="button";
    const docxButton=document.createElement("button");docxButton.textContent=languages.export+" Word";docxButton.type="button";
    const pdfButton=document.createElement("button");pdfButton.textContent=languages.export+" PDF";pdfButton.type="button";
    const safety=document.createElement("span");safety.setAttribute("role","status");
    const actions=document.createElement("button");actions.type="button";actions.className="block__icon block__icon--show";
    actions.setAttribute("aria-label",languages.more);actions.setAttribute("aria-haspopup","menu");
    actions.innerHTML='<svg><use xlink:href="#iconMore"></use></svg>';
    actions.addEventListener("click",()=>{
        const menu=window.siyuan.menus.menu;menu.remove();
        for(const button of [upload,exportButton,htmlButton,docxButton,pdfButton]){
            if(button.hidden){continue;}
            menu.append(new MenuItem({label:button.textContent,disabled:button.disabled,click:()=>button.click()}).element);
        }
        const rect=actions.getBoundingClientRect();menu.popup({x:Math.max(8,rect.right-200),y:rect.bottom});
    });
    toolbar.append(actions,picker,safety); options.element.before(toolbar);
    let selected:Range|null=null;
    toolbar.addEventListener("mousedown",event=>{event.preventDefault();selected=window.getSelection()?.rangeCount?window.getSelection().getRangeAt(0).cloneRange():null;});
    const readOnly = window.siyuan.config.readonly || window.siyuan.config.editor.readOnly;
    upload.hidden = readOnly; picker.disabled = readOnly;
    const escape=(text:string)=>text.replace(/[&<>"']/g,char=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[char]));
    let instance:Protyle;
    const canEditContent = () => !readOnly && !instance?.protyle.disabled;
    upload.onclick=()=>{if(canEditContent()){picker.click();}};
    const uploadFiles=(files:File[],range:Range|null)=>{
        // Capture the original editor and selection before any file read.
        if(!canEditContent() || !range || !options.element.contains(range.commonAncestorContainer)){return;}
        const captured=range.cloneRange();
        upload.disabled=true;picker.disabled=true;exportButton.disabled=true;htmlButton.disabled=true;docxButton.disabled=true;pdfButton.disabled=true;
        void binding.upload(files,html=>{
            if(!options.element.contains(captured.commonAncestorContainer)){throw new SessionError("upload_selection_detached");}
            const selection=window.getSelection();selection.removeAllRanges();selection.addRange(captured);
            instance.insert(html);
        },(name,label)=>genAssetHTML("."+(name.split(".").pop()||""),name,escape(label),escape(label))).catch(error=>{
            const message=document.createElement("p");message.setAttribute("role","alert");message.textContent=error instanceof Error?error.message:"upload_failed";toolbar.append(message);
        }).finally(()=>{upload.disabled=false;picker.disabled=false;exportButton.disabled=false;htmlButton.disabled=false;docxButton.disabled=false;pdfButton.disabled=false;});
    };
    picker.onchange = () => {
        if (!canEditContent() || !selected || !options.element.contains(selected.commonAncestorContainer)) {
            picker.value = "";
            return;
        }
        const selection = window.getSelection();
        selection.removeAllRanges();
        selection.addRange(selected);
        uploadProtyleFiles(instance.protyle, picker.files, picker);
    };
    const download=(format:"html"|"docj"|"docx"|"pdf")=>{
        htmlButton.disabled=true;upload.disabled=true;exportButton.disabled=true;docxButton.disabled=true;pdfButton.disabled=true;
        void (format==="html"?binding.exportHTML():format==="docx"?binding.exportDOCX():format==="pdf"?binding.exportPDF():binding.exportDocument()).then(blob=>{
            const url=URL.createObjectURL(blob),anchor=document.createElement("a");anchor.href=url;anchor.download=options.document.docId+"."+format;anchor.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
        }).catch(error=>{const message=document.createElement("p");message.setAttribute("role","alert");message.textContent=error instanceof Error?error.message:"export_failed";toolbar.append(message);})
            .finally(()=>{htmlButton.disabled=false;upload.disabled=false;exportButton.disabled=false;docxButton.disabled=false;pdfButton.disabled=false;});
    };
    exportButton.onclick=()=>download("docj");htmlButton.onclick=()=>download("html");docxButton.onclick=()=>download("docx");pdfButton.onclick=()=>download("pdf");
    const unlock = installScopedProviders({
        openDocument: id => binding.openDocument(id),
        assetURL: async path => {
            const match = /^\/?assets\/([^?#]+)(?:\?page=[1-9]\d*)?$/.exec(path);
            if (!match) { throw new SessionError("invalid_asset_path"); }
            const name = decodeURIComponent(match[1]);
            return options.session.assetUrl(name, options.document);
        },
        exportDocument: (docId, format) => {
            if (docId !== options.document.docId) { throw new SessionError("ui_export_outside_document"); }
            if (format === "html" || format === "htmlmd") { return binding.exportHTML(format === "htmlmd"); }
            if (format === "docx") { return binding.exportDOCX(); }
            return binding.exportPDF();
        },
        post: async (endpoint, data) => {
            if (/^\/api\/export\/export(?:Notebook)?(?:SY|Mds?)$/.test(endpoint)) {
                return binding.exportArchive(endpoint, data);
            }
            if (endpoint === "/api/transactions") {
                const body = data as {transactions?: {doOperations?: ProtyleOperation[]}[]};
                if (readOnly || !Array.isArray(body?.transactions) || !body.transactions.length ||
                    body.transactions.some(batch => !Array.isArray(batch?.doOperations) || !batch.doOperations.length)) {
                    throw new SessionError("invalid_ui_transaction_request");
                }
                return binding.applyOriginalProviderBatches(body.transactions.map(batch => batch.doOperations!));
            }
            if (endpoint === "/api/attr/setBlockAttrs") {
                if (window.siyuan.config.readonly || window.siyuan.config.editor.readOnly ||
                    !data || typeof data !== "object" || !("id" in data) || typeof data.id !== "string" ||
                    !("attrs" in data) || !data.attrs || typeof data.attrs !== "object" || Array.isArray(data.attrs)) {
                    throw new SessionError("invalid_ui_attribute_request");
                }
                const id = data.id;
                if (!/^\d{14}-[a-z0-9]{7}$/.test(id) ||
                    Object.values(data.attrs).some(value => typeof value !== "string" && value !== null)) {
                    throw new SessionError("invalid_ui_attribute_request");
                }
                const owner = await options.session.requestUI("/api/block/getBlockInfo", {id}) as {box?: string; rootID?: string};
                if (owner?.box !== options.document.boxId || owner?.rootID !== options.document.docId) {
                    throw new SessionError("ui_attribute_outside_document");
                }
                const operations: Array<IOperation & ProtyleOperation> = [{action: "setAttrs", id, data: data.attrs as Record<string, string | null>}];
                const history = await binding.menuTransaction(operations);
                const state = instance.protyle;
                state.undo.add(operations, [], state);
                state.undo.undoStack[state.undo.undoStack.length - 1].nativeHistory = Promise.resolve(history);
                return null;
            }
            if (isArchiveEndpoint(endpoint)) {
                if (!(data instanceof FormData) || window.siyuan.config.readonly || window.siyuan.config.editor.readOnly ||
                    !binding.canRestoreHistory) { throw new SessionError("invalid_ui_archive_request"); }
                return binding.importArchive(endpoint, data);
            }
            if (endpoint === "/api/filetree/renameDoc") {
                const title = data as {notebook?: string; path?: string; title?: string};
                if (title?.notebook === options.document.boxId && typeof title.path === "string" &&
                    title.path.endsWith("/" + options.document.docId + ".docj") && typeof title.title === "string") {
                    return binding.renameDocument({notebook: title.notebook, path: title.path, title: title.title});
                }
            }
            if (endpoint === DOC_VERSIONS_ENDPOINT) {
                const version = data as Parameters<EditorSession["restoreDocumentVersion"]>[0] & {action?: string};
                if (version?.action === "restore") { return binding.restoreDocumentVersion(version); }
                const value = await options.session.requestUI(endpoint, data);
                if (version?.action !== "preview") { return value; }
                const preview = value as {content: string; isLargeDoc: boolean};
                if (preview.isLargeDoc) { return preview; }
                return {...preview, content: await binding.prepareReadOnlyUIFragment(preview.content, version)};
            }
            const value = await options.session.requestUI(endpoint, data);
            if (endpoint === "/api/setting/setKeymap") {
                loadPortableKeymap((data as {data: Config.IKeymap}).data);
                window.siyuan.config.keymap = activePortableKeymap();
                instance.protyle.toolbar.update(instance.protyle, PORTABLE_FORMAT_TOOLS);
                return value;
            }
            if (endpoint === "/api/setting/setAppearance" || endpoint === "/api/system/setAppearanceMode") {
                const bootstrap = await options.session.requestUI("/api/system/getConf") as {conf?: UIConfiguration};
                const appearance = bootstrap.conf?.appearance;
                if (!appearance || !/^[a-z]{2}_[A-Z]{2,3}$/.test(appearance.lang)) {
                    throw new SessionError("invalid_ui_configuration");
                }
                if (appearance.lang !== window.siyuan.config.appearance.lang) {
                    const response = await fetch("/appearance/langs/" + appearance.lang + ".json", {
                        credentials: "omit", cache: "no-store", redirect: "error",
                    });
                    if (!response.ok) { throw new SessionError("editor_language_unavailable"); }
                    const languages = await response.json();
                    if (options.session.status !== "active") { throw new SessionError("stale_response"); }
                    window.siyuan.languages = languages;
                }
                applyPortableAppearance(appearance);
                setCodeTheme();
                return value;
            }
            if (endpoint === "/api/lute/html2BlockDOM") {
                if (typeof value !== "string") { throw new SessionError("invalid_ui_document"); }
                return binding.prepareUIFragment(value, options.document);
            }
            if (endpoint === "/api/block/getHeadingChildrenDOM" ||
                /^\/api\/block\/getHeading(?:Delete|Insert|Level)Transaction$/.test(endpoint)) {
                const {id} = data as {id: string};
                const loc = await options.session.requestUI("/api/block/getBlockInfo", {id}) as {box?: string; rootID?: string};
                if (!loc.box || !loc.rootID) { throw new SessionError("invalid_ui_document"); }
                const doc = {boxId: loc.box, docId: loc.rootID};
                if (typeof value === "string") { return binding.prepareUIFragment(value, doc); }
                if (value === null) { return null; }
                const plan = value as {doOperations: ProtyleOperation[]; undoOperations: ProtyleOperation[]};
                if (!Array.isArray(plan.doOperations) || !Array.isArray(plan.undoOperations)) {
                    throw new SessionError("invalid_ui_transaction_request");
                }
                const prepare = (operations: ProtyleOperation[]) => Promise.all(operations.map(async op =>
                    typeof op.data === "string" && ["insert", "update"].includes(op.action)
                        ? {...op, data: await binding.prepareUIFragment(op.data, doc)}
                        : op));
                return {doOperations: await prepare(plan.doOperations), undoOperations: await prepare(plan.undoOperations)};
            }
            if (endpoint === "/api/ref/getBacklinkDoc" || endpoint === "/api/ref/getBackmentionDoc") {
                const {refTreeID} = data as {refTreeID: string};
                const loc = await options.session.requestUI("/api/block/getBlockInfo", {id: refTreeID}) as {box?: string; rootID?: string};
                if (!loc.box || loc.rootID !== refTreeID) { throw new SessionError("invalid_ui_document"); }
                const field = endpoint === "/api/ref/getBacklinkDoc" ? "backlinks" : "backmentions";
                const result = value as { [field: string]: unknown };
                const rows = result?.[field];
                if (!Array.isArray(rows)) { throw new SessionError("invalid_ui_document"); }
                const doc = {boxId: loc.box, docId: refTreeID};
                const fragments = await Promise.all(rows.map(async row => {
                    if (typeof row?.dom !== "string") { throw new SessionError("invalid_ui_document"); }
                    return {...row, dom: await binding.prepareReadOnlyUIFragment(row.dom, doc)};
                }));
                return {...result, [field]: fragments};
            }
            if (endpoint === "/api/search/getEmbedBlock" || endpoint === "/api/search/searchEmbedBlock") {
                const result = value as {blocks?: {block: {content: string; box: string; rootID: string}}[]};
                if (!Array.isArray(result?.blocks)) { throw new SessionError("invalid_ui_document"); }
                const blocks = await Promise.all(result.blocks.map(async row => {
                    const block = row?.block;
                    if (typeof block?.content !== "string" || !block.box || !block.rootID) {
                        throw new SessionError("invalid_ui_document");
                    }
                    const content = await binding.prepareReadOnlyUIFragment(block.content, {boxId: block.box, docId: block.rootID});
                    return {...row, block: {...block, content}};
                }));
                return {blocks};
            }
            const field = endpoint === "/api/block/getBlockDOM" ? "dom" :
                ["/api/filetree/getDoc", "/api/history/getDocHistoryContent", "/api/template/render"].includes(endpoint) ? "content" : "";
            if (field) {
                const result = value as Record<string, unknown>;
                if (typeof result?.[field] !== "string") { throw new SessionError("invalid_ui_document"); }
                let doc = options.document;
                if (endpoint === "/api/filetree/getDoc" && typeof result.box === "string" && typeof result.rootID === "string") {
                    doc = {boxId: result.box, docId: result.rootID};
                } else if (endpoint === "/api/block/getBlockDOM" && typeof result.id === "string") {
                    const loc = await options.session.requestUI("/api/block/getBlockInfo", {id: result.id}) as {box?: string; rootID?: string};
                    if (!loc.box || !loc.rootID) { throw new SessionError("invalid_ui_document"); }
                    doc = {boxId: loc.box, docId: loc.rootID};
                }
                if (endpoint === "/api/history/getDocHistoryContent") {
                    if (result.isLargeDoc === true) { return value; }
                    const path = (data as {historyPath?: string})?.historyPath?.split("/") ?? [];
                    const docId = path.at(-1)?.replace(/\.docj$/, "");
                    const nativeID = /^\d{14}-[a-z0-9]{7}$/;
                    if (!nativeID.test(path[1] ?? "") || !nativeID.test(docId ?? "")) { throw new SessionError("invalid_ui_document"); }
                    return {...result, [field]: await binding.prepareReadOnlyUIFragment(result[field] as string, {boxId:path[1], docId:docId!})};
                }
                return {...result, [field]: await binding.prepareUIFragment(result[field] as string, doc)};
            }
            return value;
        },
        get: async url => {
            if (!/^\/appearance\/langs\/[a-z]{2}_[A-Z]{2,3}\.json(?:\?v=[a-zA-Z0-9._-]+)?$/.test(url)) {
                throw new SessionError("ui_resource_unavailable");
            }
            const response = await fetch(url, {credentials: "omit", cache: "no-store", redirect: "error"});
            if (!response.ok) { throw new SessionError("ui_resource_unavailable"); }
            return response.json();
        },
        subscribe: listener => options.session.onWorkspaceEvent(listener),
        error: error => { safety.textContent = error instanceof Error ? error.message : "ui_request_failed"; },
    });
    binding.onDetach(unlock);
    binding.onDetach(()=>toolbar.remove());
    try {
        await binding.subscribe();
        instance=new Protyle(options.app,options.element,{
            portable:binding,blockId:options.document.docId,
            toolbar:[...PORTABLE_FORMAT_TOOLS],
            action:[Constants.CB_GET_ALL],
            render:{title:true,background:true,breadcrumb:!single,scroll:false,gutter:true},
            hint:{extend:[...[...Constants.BLOCK_HINT_KEYS, ...Constants.BLOCK_EMBED_HINT_KEYS].map(key => ({key})),
                {key:"/"}, {key:"、"}, {key:":"}]},
            preview:{actions:["desktop", "tablet", "mobile", {
                key:"wysiwyg", className:"", text:window.siyuan.languages.wysiwyg,
                click: () => setEditMode(instance.protyle, "wysiwyg"),
            }]},
            upload:{url:"",handler:files=>{const selection=window.getSelection();uploadFiles(files,selection?.rangeCount?selection.getRangeAt(0):null);return null;}},
        });
        const state=instance.protyle;
        const paneResize=new ResizeObserver(()=>instance.resize());
        paneResize.observe(layout.pane);
        binding.onDetach(()=>paneResize.disconnect());
        const externalPreview = (event: MouseEvent) => {
            const preview = (event.target as Element)?.closest?.<HTMLElement>("[data-wiki-preview]");
            if (!preview || !state.wysiwyg.element.contains(preview)) { return; }
            event.preventDefault();
            event.stopImmediatePropagation();
            void showExternalPreview(binding, preview.dataset.wikiPreview as ExternalKind | "widget", preview.dataset.wikiPreviewUrl, preview);
        };
        state.wysiwyg.element.addEventListener("click", externalPreview, true);
        binding.onDetach(() => state.wysiwyg.element.removeEventListener("click", externalPreview, true));
        binding.onDetach(() => state.preview.clearScoped());
        binding.setHeaderFlush(() => state.title.flushRename());
        binding.onDetach(() => state.title.cancelPendingRename());
        const colorScheme = window.matchMedia("(prefers-color-scheme: dark)");
        const onColorScheme = () => {
            if (!window.siyuan.config.appearance.modeOS) { return; }
            applyPortableAppearance(window.siyuan.config.appearance);
            setCodeTheme();
        };
        colorScheme.addEventListener("change", onColorScheme);
        binding.onDetach(() => colorScheme.removeEventListener("change", onColorScheme));
        let configurationGeneration = 0;
        binding.onDetach(options.session.onWorkspaceEvent(event => {
            if (event.type !== "workspaceEvent" || !(event.effect as {configuration?: boolean})?.configuration) { return; }
            const generation = ++configurationGeneration;
            void options.session.requestUI("/api/system/getConf").then(value => {
                if (generation !== configurationGeneration || options.session.status !== "active") { return; }
                const cfg = (value as {conf?: UIConfiguration})?.conf;
                if (!cfg) { return; }
                Object.assign(window.siyuan.config.editor, cfg.editor, {allowHTMLBLockScript: false});
                Object.assign(window.siyuan.config.fileTree, cfg.fileTree);
                Object.assign(window.siyuan.config.flashcard, cfg.flashcard);
                instance.resize();
            }).catch(() => undefined);
        }));
        binding.onDetach(installScopedBlockPopover(options.app, state.wysiwyg.element, async id => {
            const target = await options.session.requestUI("/api/block/getBlockInfo", {id}) as {box?: string; rootID?: string};
            if (!target?.box || !target.rootID) { throw new SessionError("invalid_ui_document"); }
            const result = await options.session.requestUI("/api/block/getBlockDOM", {id}) as {dom?: string};
            if (typeof result?.dom !== "string") { throw new SessionError("invalid_ui_document"); }
            return binding.prepareReadOnlyUIFragment(result.dom, {boxId: target.box, docId: target.rootID});
        }));
        const searchFromBlock = (keyword: string, block?: Element) => {
            void import("../search/scoped").then(({openScopedSearch}) => {
                if (state.element.isConnected && (!block || block.isConnected)) { openScopedSearch(binding, keyword); }
            }).catch(() => { safety.textContent = window.siyuan.languages.operationFailed; });
        };
        let pdfDialog: Dialog | undefined;
        let tagDialog: Dialog | undefined;
        binding.onDetach(() => {
            if (pdfDialog?.element.classList.contains("b3-dialog--open")) { pdfDialog.destroy(); }
            if (tagDialog?.element.classList.contains("b3-dialog--open")) { tagDialog.destroy(); }
        });
        const openTagRename = (label: string) => {
            if (canEditContent() && !tagDialog?.element.isConnected) {
                tagDialog = renameTag(label, newLabel => binding.renameTag(label, newLabel));
            }
        };
        const openAnnotation = (reference: string) => {
            const parts = /^(assets\/(?:[^/]+\/)*[^/]+\.pdf)\/(\d{14}-[a-z0-9]{7})$/i.exec(reference);
            if (!parts || parts[1].split("/").some(part => part === "." || part === "..") ||
                /[\\\x00?#]|%(?:2e|2f|5c|00)/i.test(parts[1])) {
                safety.textContent = window.siyuan.languages.operationFailed;
                return;
            }
            if (pdfDialog) { return; }
            void import("../asset").then(({Asset}) => {
                if (pdfDialog || !state.element.isConnected) { return; }
                let viewer: InstanceType<typeof Asset> | undefined;
                const dialog = pdfDialog = new Dialog({
                    title:"PDF", width:"min(94vw, 1100px)", height:"85vh", disableAnimation:true,
                    content:'<div data-scoped-pdf-annotation style="height:100%;position:relative"></div>',
                    destroyCallback:() => { viewer?.dispose(); pdfDialog = undefined; },
                });
                const panel = dialog.element.querySelector<HTMLElement>("[data-scoped-pdf-annotation]");
                const fail = () => {
                    if (!panel.isConnected) { return; }
                    viewer?.dispose();
                    panel.textContent = window.siyuan.languages.operationFailed;
                    panel.setAttribute("role", "alert");
                };
                viewer = new Asset({app:options.app, path:parts[1], page:parts[2],
                    tab:{id:crypto.randomUUID(), headElement:dialog.element, panelElement:panel}, onError:fail});
            }).catch(() => { safety.textContent = window.siyuan.languages.operationFailed; });
        };
        binding.onDetach(installPortableMenu({root:state.element,content:state.wysiwyg.element,nativeBlockActions:true,
            contentMenu:(event, block)=>{
                contentMenu(state, block);
                window.siyuan.menus.menu.popup({x:event.clientX, y:event.clientY});
            },
            blockAttribute:(event, block, field)=>{
                const marker = block.querySelector(".protyle-attr--" + field);
                const keyword = (field === "memo" ? marker?.getAttribute("aria-label") : marker?.textContent)?.trim();
                if (!single && isOnlyMeta(event) && keyword) {
                    searchFromBlock(keyword, block);
                    return;
                }
                openAttr(block, field, state);
            },
            inlineMemo:memo=>state.toolbar.showRender(state, memo),
            openAnnotation,
            searchTag:single ? undefined : label=>searchFromBlock("#" + label + "#"),
            tagMenu:single ? undefined : (event, label) => {
                const menu = new Menu("scoped-body-tag");
                if (menu.isOpen) { return; }
                menu.addItem({id:"search", icon:"iconSearch", label:window.siyuan.languages.search,
                    click:() => searchFromBlock("#" + label + "#")});
                if (canEditContent()) {
                    menu.addItem({id:"rename", icon:"iconEdit", label:window.siyuan.languages.rename,
                        click:() => openTagRename(label)});
                }
                menu.open({x:event.clientX, y:event.clientY});
            },
            codeOptions:(block,trigger)=>{
                const id = block.dataset.nodeId;
                if (!/^\d{14}-[a-z0-9]{7}$/.test(id ?? "")) { return; }
                const menu = new Menu("scoped-code-display");
                if (menu.isOpen) { return; }
                for (const option of codeBlockOptions(block, (key, value) => {
                    if (block.isConnected && !state.disabled && binding.canRestoreHistory) {
                        fetchPost("/api/attr/setBlockAttrs", {id, attrs:{[key]:value}});
                    }
                })) {
                    menu.addItem(option);
                }
                const rect = trigger.getBoundingClientRect();
                menu.open({x:rect.left,y:rect.top,isLeft:true});
            },
            editable:()=>!state.disabled&&binding.canRestoreHistory,
            canUndo:()=>state.undo.undoStack.length>0,canRedo:()=>state.undo.redoStack.length>0,
            undo:()=>state.undo.undo(state),redo:()=>state.undo.redo(state),
            notice:message=>{safety.textContent=message;},
            remove:block=>{
                const operations:Array<IOperation & ProtyleOperation>=[{action:"delete",id:block.dataset.nodeId!}];
                if(state.wysiwyg.element.querySelectorAll(":scope > [data-node-id]").length===1){
                    const id=Lute.NewNodeID();
                    operations.push({action:"insert",id,parentID:binding.document.docId,data:`<div data-node-id="${id}" data-type="NodeParagraph" class="p"><div contenteditable="true"></div><div class="protyle-attr" contenteditable="false"></div></div>`});
                }
                // Undo uses native snapshots only; no inverse DOM operation is
                // executed. Register synchronously before the async save.
                state.undo.add(operations,[],state);
                const entry=state.undo.undoStack[state.undo.undoStack.length-1];
                entry.nativeHistory=binding.menuTransaction(operations);
                void entry.nativeHistory.catch(()=>undefined);
            },
        }));
        const mounted=instance;
        binding.onDetach(()=>mounted.destroy());
        await instance.portableReady;
        void mountScopedPlugins(binding, options.element, toolbar, state.wysiwyg.element, layout, options.hostPluginLayout)
            .catch(() => binding.reportUnavailableTool());
        let updateTabTitle: (title: string) => void;
        if (!single) {
            const tabs = mountScopedTabs(layout.workspace, layout.pane, options.app, message => { safety.textContent = message; },
                () => ({box:binding.document.boxId,path:state.path}));
            binding.tabs = tabs;
            updateTabTitle = tabs.setTitle;
            binding.onDetach(tabs.destroy);
        }
        await mountPortableLayout(layout.sidebar, binding, state, options.app, message => { safety.textContent = message; }, updateTabTitle, openTagRename);
    } catch(error){binding.reject(error);binding.detach();throw error;}
    return {protyle:instance,binding,dispose:async()=>{await binding.prepareRelease();binding.detach();}};
}
