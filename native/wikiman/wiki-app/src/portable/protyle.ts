import {ArchiveEndpoint, AttributeCellVersion, DocumentKey, EditorSession, NativeHistory, ProtyleOperation, SessionError, recoverableUIValidation} from "./session";
import {referenceTarget} from "./reference";
import {externalControl, externalURL} from "./external";
import type {PluginHostAction} from "./bridge";

/** Structural boundary for the actual Protyle instance. Tests can exercise its
 * lifecycle without mocking a successful native save or mounting a text area. */
export interface ProtylePort {
    element: HTMLElement;
    render(html: string, path?: string): void;
    setDisabled(disabled: boolean): void;
    clearHistory(): void;
    historyCommand?(redo: boolean): void;
    hasOverlayDraft?(): boolean;
    savePosition?(): Promise<void>;
    error(error: unknown): void;
}

export class ProtyleBinding {
    readonly document: Readonly<DocumentKey>;
    pluginSystemShortcuts: string[] = [];
    reportPluginSystemShortcuts?: (chords: string[]) => void;
    runPluginSystemShortcut?: (chord: string) => void;
    pluginHostActions: PluginHostAction[] = [];
    reportPluginHostActions?: (actions: PluginHostAction[]) => void;
    runPluginHostAction?: (id: string) => void;
    aiWritingEnabled = false;
    widgetEnabled = false;
    private port: ProtylePort | null = null;
    private epoch = 0;
    private composing = false;
    private rendering = false;
    private frozen = false;
    private failure: unknown = null;
    private uploading = false;
    private clipboardOperations: ProtyleOperation[] | null = null;
    private exporting = false;
    private restoring = false;
    private refreshing = false;
    private sourceRefresh: Promise<void> | null = null;
    private attributeCellDraft = false;
    private refreshAgain = false;
    private pendingInput: {blockId: string; commit: () => void; timer: number} | null = null;
    private navigateReference: ((blockId:string,reuseCurrent?:boolean)=>Promise<void>) | null = null;
    private renderedHash: string | null = null;
    private readonly assets = new Map<string, string>();
    private flushHeader?: () => Promise<void>;
    private cleanup: Array<() => void> = [];
    constructor(readonly session: EditorSession, doc: DocumentKey) { this.document = Object.freeze({...doc}); }
    onDetach(cleanup: () => void): void { this.cleanup.push(cleanup); }
    setHeaderFlush(flush: () => Promise<void>): void { this.flushHeader = flush; }
    /** Unavailable UI tools are not save failures. Keep input/queues alive. */
    reportUnavailableTool():void {
        if(this.port&&!this.frozen&&this.session.status==="active"){this.port.error(new SessionError("portable_tool_unavailable"));}
    }
    setReferenceNavigator(navigate:((blockId:string,reuseCurrent?:boolean)=>Promise<void>)|null):void { this.navigateReference=navigate; }
    async openDocument(id: string,reuseCurrent=false): Promise<void> {
        this.active();
        if (id === this.document.docId) { return; }
        this.flushPendingInput();
        await this.flushHeader?.();
        if (!this.canRestoreHistory) { throw new SessionError("unsaved_editor_state"); }
        if (!this.navigateReference) { throw new SessionError("reference_requires_scoped_resolution"); }
        await this.navigateReference(id,reuseCurrent);
    }
    originalAssetPath(url: string): string | undefined {
        return this.assets.get(url);
    }
    /** Focus only the exact source version resolved by the host. */
    focusReference(blockId:string,expectedHash?:string,focus=true):void {
        const port=this.active();
        if(expectedHash && expectedHash!==this.renderedHash){throw new SessionError("reference_source_changed");}
        const targets=Array.from(port.element.querySelectorAll<HTMLElement>("[data-node-id]")).filter(node=>node.dataset.nodeId===blockId && !node.closest(".protyle-wysiwyg__embed"));
        if(targets.length!==1){throw new SessionError(targets.length?"ambiguous_block_reference":"reference_not_rendered");}
        if(!focus){return;}
        const destination=targets[0];destination.scrollIntoView?.({block:"center"});
        const editable=destination.matches('[contenteditable="true"]')?destination:destination.querySelector<HTMLElement>('[contenteditable="true"]');
        if(editable){
            // Protyle has a contenteditable WYSIWYG root with nested editable
            // blocks. Browsers focus the editing host, not its nested child.
            const focusHost=port.element.matches('[contenteditable="true"]')?port.element:editable;
            focusHost.focus({preventScroll:true});
            const range=port.element.ownerDocument.createRange();range.selectNodeContents(editable);range.collapse(true);
            const selection=port.element.ownerDocument.getSelection();selection?.removeAllRanges();selection?.addRange(range);
            if(port.element.ownerDocument.activeElement!==focusHost){throw new SessionError("reference_keyboard_focus_unavailable");}
        }
    }
    attach(port: ProtylePort): void {
        if (this.port) { throw new SessionError("editor_already_attached"); }
        this.port = port;
        const input = () => { if (!this.rendering && !this.frozen) { try { this.session.markInputDirty(true); } catch (error) { this.reject(error); } } };
        const isFormInput = (event: Event) => event.target instanceof Element && event.target.matches("input, textarea, select");
        const start = (event: Event) => { if (!isFormInput(event)) { this.flushPendingInput(); this.composing = true; input(); } };
        const end = (event: Event) => { if (!isFormInput(event)) { this.composing = false; } };
        const beforeInput = (event: InputEvent) => {
            if (isFormInput(event)) { return; }
            // WKWebView's Edit menu can deliver the native history command
            // directly, without a keydown in the iframe.
            if (event.cancelable && port.historyCommand && ["historyUndo", "historyRedo"].includes(event.inputType)) {
                event.preventDefault();
                if (!this.frozen && this.port === port) { port.historyCommand(event.inputType === "historyRedo"); }
                return;
            }
            input();
        };
        port.element.addEventListener("beforeinput", beforeInput, true);
        port.element.addEventListener("compositionstart", start, true);
        port.element.addEventListener("compositionend", end, true);
        const download = (event: MouseEvent) => {
            const reference=referenceTarget(event.target as Element,port.element);
            if(reference){
                event.preventDefault();event.stopImmediatePropagation();
                if(this.port!==port||this.frozen||this.session.status!=="active"){return;}
                if(!this.canRestoreHistory){port.error(new SessionError("unsaved_editor_state"));return;}
                const targets=reference.blockId?Array.from(port.element.querySelectorAll<HTMLElement>("[data-node-id]")).filter(node=>node.dataset.nodeId===reference.blockId && !node.closest(".protyle-wysiwyg__embed")):[];
                if(!reference.blockId){port.error(new SessionError("invalid_block_reference"));return;}
                if(targets.length>1){port.error(new SessionError("ambiguous_block_reference"));return;}
                if(targets.length===1){
                    try{this.focusReference(reference.blockId);}
                    catch(error){if(!(error instanceof SessionError && error.code==="reference_keyboard_focus_unavailable")){port.error(error);}}
                    return;
                }
                const navigate=this.navigateReference,epoch=this.epoch;
                if(!navigate){port.error(new SessionError("reference_requires_scoped_resolution"));return;}
                void navigate(reference.blockId).catch(error=>{if(this.port===port&&!this.frozen&&epoch===this.epoch){port.error(error);}});
                return;
            }
            const link = (event.target as Element)?.closest?.("[data-href],a[href]");
            const value = link?.getAttribute("data-href") ?? link?.getAttribute("href");
            const name = value?.startsWith("assets/") ? value.slice(7) : value?.startsWith("/assets/") ? value.slice(8) : null;
            if (name === null) { return; }
            event.preventDefault(); event.stopImmediatePropagation();
            const epoch = this.epoch;
            let decoded:string;
            try { decoded=decodeURIComponent(name); } catch { port.error(new SessionError("invalid_asset_path")); return; }
            void this.session.assetUrl(decoded, this.document).then(url => {
                if (epoch !== this.epoch || this.frozen || this.port !== port) { return; }
                const anchor = document.createElement("a"); anchor.href=url; anchor.download=name.split("/").pop() || "attachment"; anchor.click();
            }).catch(error => port.error(error));
        };
        port.element.addEventListener("click", download, true);
        this.cleanup.push(() => {
            port.element.removeEventListener("click", download, true);
            port.element.removeEventListener("beforeinput", beforeInput, true);
            port.element.removeEventListener("compositionstart", start, true);
            port.element.removeEventListener("compositionend", end, true);
        }, this.session.onStale(() => { this.cancelPendingInput(); this.frozen = true; this.epoch++; port.setDisabled(true); port.clearHistory(); port.error(new SessionError("editor_session_stale")); }));
    }
    private active(): ProtylePort {
        if (!this.port || this.frozen || this.session.status !== "active") { throw new SessionError("editor_not_active"); }
        return this.port;
    }
    reject(error: unknown): void {
        this.cancelPendingInput();
        this.failure = error; this.frozen = true; this.epoch++; this.session.fail(error);
        this.port?.setDisabled(true); this.port?.error(error);
    }
    async load(clearHistory = true): Promise<void> {
        const port = this.active(), epoch = ++this.epoch;
        port.setDisabled(true);
        if (clearHistory) { port.clearHistory(); }
        try {
            const rendered = await this.session.renderDocument(this.document);
            const html = await this.prepareAssets(rendered.html);
            if (epoch !== this.epoch || this.frozen || this.port !== port) { throw new SessionError("stale_response"); }
            this.rendering = true;
            try { port.render(html, rendered.path); } finally { this.rendering = false; }
            this.renderedHash=rendered.hash;
            port.setDisabled(false);
        } catch (error) { if (epoch === this.epoch && this.port === port) { this.reject(error); } throw error; }
    }
    subscribe(): Promise<void> {
        return this.session.subscribe(event => {
            if (event.type !== "workspaceEvent") { return; }
            const operation = (event.event as {operationId?: string})?.operationId;
            if (operation && this.session.ownsOperation(operation)) { return; }
            const effect = event.effect as {documents?: string[]; attributeViews?: string[]; other?: boolean} | undefined;
            if (effect?.other === false && Array.isArray(effect.documents) && !effect.documents.includes(this.document.docId)) {
                const visibleAV = Array.from(this.port?.element.querySelectorAll<HTMLElement>("[data-av-id]") ?? []);
                if (!visibleAV.some(element => effect.attributeViews?.includes(element.dataset.avId ?? ""))) { return; }
            }
            if (effect?.other === false && Array.isArray(effect.documents)) {
                if (this.refreshing) {
                    this.refreshAgain = true;
                    return;
                }
                if (this.canRestoreHistory && !this.session.dirty) {
                    this.sourceRefresh = this.refreshCommittedDocument().finally(() => {this.sourceRefresh=null;});
                    return;
                }
            }
            this.session.invalidate();
        });
    }
    private async refreshCommittedDocument(): Promise<void> {
        this.refreshing = true;
        try {
            do {
                this.refreshAgain = false;
                await this.load();
            } while (this.refreshAgain && !this.frozen && this.session.status === "active");
        } catch {
            // load reports errors to this editor and disables the stale DOM.
        } finally {
            this.refreshing = false;
        }
    }
    bufferInput(blockId: string, delay: number, commit: () => void): void {
        this.active();
        if (this.pendingInput?.blockId === blockId) {
            this.pendingInput.commit = commit;
        } else {
            this.flushPendingInput();
            const timer = window.setTimeout(() => this.flushPendingInput(), delay);
            this.pendingInput = {blockId, commit, timer};
        }
        this.session.markInputDirty(true);
    }
    flushPendingInput(): void {
        const pending = this.pendingInput;
        if (!pending) { return; }
        this.cancelPendingInput();
        if (!this.frozen && this.session.status === "active") { pending.commit(); }
    }
    private cancelPendingInput(): void {
        if (this.pendingInput) { window.clearTimeout(this.pendingInput.timer); this.pendingInput = null; }
    }
    async transact(operations: readonly ProtyleOperation[], captureHistory = false): Promise<NativeHistory | undefined> {
        const port = this.active();
        let refresh = false;
        try {
            operations = operations.map(op => {
                if ((op.action === "foldHeading" || op.action === "unfoldHeading") && "context" in op) {
                    const {context, ...operation} = op;
                    if (context && typeof context === "object" && Object.keys(context).every(key => key === "focusId")) {
                        return operation;
                    }
                }
                return op;
            });
            const keys = new Set(["action", "id", "data", "parentID", "previousID", "nextID"]);
            const actions = new Set(["update", "insert", "delete", "move", "moveOutlineHeading", "append", "appendInsert", "prependInsert", "setAttrs", "foldHeading", "unfoldHeading", "doUpdateUpdated"]);
            if (operations.some(op => !actions.has(op.action) &&
                !/^(?:addFlashcards|removeFlashcards|[a-z]+AttrView[A-Za-z]+)$/.test(op.action))) {
                throw new SessionError("richtext_operation_unsupported");
            }
            const advanced = operations.some(op => !actions.has(op.action) ||
                Object.entries(op).some(([key, value]) => value !== undefined && !keys.has(key)));
            if (advanced && this.session.view.mode !== "wikiDesktop") { throw new SessionError("richtext_operation_unsupported"); }
            const captured = operations.map(op => {
                return {...op, ...(typeof op.data === "string" && ["insert", "update", "append", "appendInsert", "prependInsert"].includes(op.action) ? {data: this.restoreAssetReferences(op.data)} : {})};
            });
            refresh = !this.restoring && captured.some(op => ["foldHeading", "unfoldHeading", "setAttrs"].includes(op.action));
            if (refresh) {
                port.element.setAttribute("aria-busy", "true");
                port.setDisabled(true);
            }
            const work = advanced ? this.session.transactOriginal(this.document, [captured]).then(result => captureHistory ? result.history : undefined) :
                this.session.transact(this.document, captured, captureHistory);
            if (!this.composing) { this.session.markInputDirty(false); }
            const history = await work;
            if (refresh && this.port === port && !this.frozen) { await this.session.flush(); await this.load(false); }
            return history;
        } catch (error) { this.reject(error); throw error; }
        finally { if (refresh) { port.element.removeAttribute("aria-busy"); } }
    }
    async applyOriginalProviderBatches(batches: readonly (readonly ProtyleOperation[])[]): Promise<unknown> {
        return (await this.commitOriginalBatches(batches)).transactions;
    }
    async updateAttributeView(operations: readonly ProtyleOperation[], version: AttributeCellVersion): Promise<NativeHistory> {
        return (await this.commitOriginalBatches([operations], version)).history;
    }
    private async commitOriginalBatches(batches: readonly (readonly ProtyleOperation[])[], cellVersion?: AttributeCellVersion) {
        const port = this.active();
        this.flushPendingInput();
        if (this.session.view.mode !== "wikiDesktop") { throw new SessionError("richtext_operation_unsupported"); }
        if (!this.canRestoreHistory) { throw new SessionError("unsaved_editor_state"); }
        this.restoring = true;
        port.setDisabled(true);
        port.element.setAttribute("aria-busy","true");
        try {
            const captured = batches.map(operations => operations.map(op => ({...op,
                ...(typeof op.data === "string" && ["insert", "update", "append", "appendInsert", "prependInsert"].includes(op.action)
                    ? {data: this.restoreAssetReferences(op.data)} : {}),
            })));
            const result = cellVersion ? await this.session.transactOriginal(this.document, captured, true, cellVersion) :
                await this.session.transactOriginal(this.document, captured, true);
            await this.load(!cellVersion);
            return result;
        } catch (error) {
            if (recoverableUIValidation(error, !!cellVersion)) {
                if (error instanceof SessionError && error.code === "source_conflict") { await this.load(); }
                else { port.setDisabled(false); }
            }
            else { this.reject(error); }
            throw error;
        } finally { this.restoring = false; port.element.removeAttribute("aria-busy"); }
    }
    async importArchive(endpoint: ArchiveEndpoint, form: FormData): Promise<void> {
        await this.applyScopedChange(() => this.session.importArchive(endpoint, form));
    }
    async renameDocument(data: {notebook: string; path: string; title: string}): Promise<void> {
        await this.applyScopedChange(() => this.session.renameDocument(this.document, data));
    }
    async createDocumentReference(data: Parameters<EditorSession["createDocumentReference"]>[1]): Promise<void> {
        const captured = {...data, blockDOM: this.restoreAssetReferences(data.blockDOM)};
        await this.applyScopedChange(() => this.session.createDocumentReference(this.document, captured));
        await this.openDocument(data.id);
    }
    async renameTag(oldLabel: string, newLabel: string): Promise<void> {
        await this.applyScopedChange(() => this.session.renameTag(oldLabel, newLabel));
    }
    async restoreDocumentVersion(version: Parameters<EditorSession["restoreDocumentVersion"]>[0]): Promise<void> {
        await this.applyScopedChange(() => this.session.restoreDocumentVersion(version),
            version.boxId === this.document.boxId && version.docId === this.document.docId);
    }
    private async applyScopedChange(write: () => Promise<void>, reload = true): Promise<void> {
        const port = this.active();
        this.flushPendingInput();
        if (!this.canRestoreHistory) { throw new SessionError("unsaved_editor_state"); }
        this.restoring = true;
        port.setDisabled(true);
        try {
            await this.session.flush();
            await write();
            // The original operation may update the mounted document and its links.
            if (reload) { await this.load(true); }
            else { port.setDisabled(false); }
        } catch (error) {
            if (this.session.status === "active" && !this.frozen) { port.setDisabled(false); }
            else { this.reject(error); }
            throw error;
        } finally { this.restoring = false; }
    }
    get canRestoreHistory(): boolean {
        return !!this.port && !this.frozen && !this.failure && !this.restoring && !this.refreshing && !this.composing && !this.uploading && !this.exporting &&
            !this.attributeCellDraft && !this.port.hasOverlayDraft?.() && !this.session.unconfirmedInput && this.session.status === "active";
    }
    setAttributeCellDraft(open: boolean): void { this.attributeCellDraft = open; }
    handleFocusedHistory(redo: boolean): boolean {
        const port = this.port;
        const focused = port?.element.ownerDocument.activeElement;
        if (!port?.historyCommand || !focused || !port.element.contains(focused) ||
            focused !== port.element &&
            focused.matches("input, textarea, select, button, [contenteditable=false]:not([data-portable-av-editor])")) {
            return false;
        }
        if (!this.frozen && !this.composing) { port.historyCommand(redo); }
        return true;
    }
    /** Menu edits use the same native history/CAS queue, then render committed
     * source. No optimistic DOM removal or inverse HTML reconstruction. */
    async menuTransaction(operations: readonly ProtyleOperation[]): Promise<NativeHistory | undefined> {
        const port=this.active();
        this.flushPendingInput();
        if(!this.canRestoreHistory){throw new SessionError("unsaved_editor_state");}
        this.restoring=true;port.setDisabled(true);port.element.setAttribute("aria-busy","true");
        try{
            const history=await this.transact(operations,true);
            await this.session.flush();
            if(this.port!==port||this.frozen){throw new SessionError("stale_response");}
            await this.load(false);
            const editable=port.element.querySelector<HTMLElement>('[contenteditable="true"]');
            if (editable && !port.element.ownerDocument.activeElement?.closest(".b3-dialog")) {
                const focusHost = port.element.matches('[contenteditable="true"]') ? port.element : editable;
                focusHost.focus({preventScroll:true});
                const range=port.element.ownerDocument.createRange();range.selectNodeContents(editable);range.collapse(false);
                const selection=port.element.ownerDocument.getSelection();selection?.removeAllRanges();selection?.addRange(range);
            }
            return history;
        }catch(error){this.reject(error);throw error;}
        finally{this.restoring=false;port.element.removeAttribute("aria-busy");}
    }
    async restoreHistory(history: Promise<NativeHistory | undefined>, redo: boolean): Promise<void> {
        const port = this.active();
        if (this.restoring || this.composing || this.uploading || this.exporting || this.session.unconfirmedInput) { throw new SessionError("unsaved_editor_state"); }
        this.restoring = true;
        port.element.setAttribute("aria-busy", "true");
        const selection = port.element.ownerDocument.getSelection();
        const anchor = selection?.anchorNode;
        const anchorElement = anchor?.nodeType === 1 ? anchor as Element : anchor?.parentElement;
        const blockId = anchorElement?.closest("[data-node-id]")?.getAttribute("data-node-id");
        port.setDisabled(true);
        try {
            const snapshot = await history;
            if (!snapshot || this.port !== port || this.frozen) { throw new SessionError("stale_native_history"); }
            await this.session.restoreHistory(this.document, snapshot, redo);
            await this.session.flush();
            if (this.port !== port || this.frozen) { throw new SessionError("stale_native_history"); }
            await this.load(false);
            // Replacing source DOM detaches the old focused editable. Keep
            // keyboard undo/redo in this editor, never in another page/window.
            const block = blockId ? Array.from(port.element.querySelectorAll<HTMLElement>("[data-node-id]")).find(node => node.dataset.nodeId === blockId) : undefined;
            const editable = block?.querySelector<HTMLElement>('[contenteditable="true"]') ?? port.element.querySelector<HTMLElement>('[contenteditable="true"]');
            const focusHost = port.element.matches('[contenteditable="true"]') ? port.element : editable;
            focusHost?.focus({preventScroll: true});
            if (editable) {
                const range = port.element.ownerDocument.createRange(); range.selectNodeContents(editable); range.collapse(false);
                selection?.removeAllRanges(); selection?.addRange(range);
            }
        } catch (error) { this.reject(error); throw error; }
        finally { this.restoring = false; port.element.removeAttribute("aria-busy"); }
    }
    async prepareRelease(): Promise<void> {
        if (!this.port) { throw new SessionError("editor_not_attached"); }
        this.flushPendingInput();
        // A failed preflight must leave IME/input alive so the user can finish
        // composition and retry. Never acknowledge or silently discard it.
        await this.flushHeader?.();
        if (this.restoring || this.refreshing || this.exporting || this.uploading || this.composing || this.attributeCellDraft ||
            this.port.hasOverlayDraft?.() || this.failure || this.session.unconfirmedInput) { throw new SessionError("unsaved_editor_state"); }
        this.frozen = true; this.epoch++; this.port.setDisabled(true);
        await this.port.savePosition?.().catch(() => undefined);
        await this.session.prepareRelease(); this.port.clearHistory();
    }
    /** Switching tabs keeps this lease, DOM and undo stack alive. */
    async prepareTabSwitch(): Promise<void> {
        this.flushPendingInput();
        await this.flushHeader?.();
        await this.sourceRefresh;
        if (!this.canRestoreHistory) { throw new SessionError("unsaved_editor_state"); }
        await this.session.flush();
        await this.sourceRefresh;
        if (!this.canRestoreHistory) { throw new SessionError("unsaved_editor_state"); }
    }
    focusTab(): void {
        const port = this.active();
        const root = port.element;
        if (root.matches('[contenteditable="true"]')) {
            const selection = root.ownerDocument.getSelection();
            const range = selection?.rangeCount && root.contains(selection.anchorNode) ? selection.getRangeAt(0).cloneRange() : undefined;
            root.focus({preventScroll:true});
            if (range && selection) { selection.removeAllRanges(); selection.addRange(range); }
        }
    }
    /** Detach is not a save/discard acknowledgement. Dirty queues remain dirty
     * in their original session and the host must retain/drain that owner. */
    detach(): void {
        this.cancelPendingInput();
        this.navigateReference=null;
        this.frozen = true; this.epoch++;
        const port=this.port;this.port=null;
        port?.setDisabled(true); port?.clearHistory();
        for (const cleanup of this.cleanup.splice(0)) { cleanup(); }
    }
    async upload(files: readonly File[], insert: (html:string) => void, assetHTML: (name:string,label:string) => string): Promise<void> {
        const port=this.active(), epoch=this.epoch;
        this.flushPendingInput();
        if (this.restoring || this.refreshing || this.exporting || this.uploading || this.composing || this.session.unconfirmedInput || !files.length) { throw new SessionError("unsaved_editor_state"); }
        if (files.length > 20 || files.some(file=>file.size > 8*1024*1024)) { throw new SessionError("asset_size_limit"); }
        this.uploading=true; port.setDisabled(true);
        try {
            await this.session.flush();
            this.session.markInputDirty(true);
            let html="";
            for (const file of files) {
                const name=await this.session.putAsset(file,file.name);
                html+=await this.prepareAssets(assetHTML("assets/"+name,file.name));
            }
            if (this.port!==port || this.frozen || epoch!==this.epoch) { throw new SessionError("stale_response"); }
            // Insertion must enter the normal native transaction/undo pipeline.
            // Protyle rejects transactions while disabled. Re-enable only in
            // this synchronous insertion turn; upload preflight still blocks
            // release until the document queue has actually drained.
            port.setDisabled(false);
            insert(html);
            this.flushPendingInput();
            await this.session.flush();
            port.setDisabled(false);
        } catch(error) { this.reject(error); throw error; }
        finally { this.uploading=false; }
    }
    captureClipboardOperations(operations: readonly ProtyleOperation[]): boolean {
        if (!this.clipboardOperations) { return false; }
        this.clipboardOperations.push(...operations);
        return true;
    }
    async pasteMedia(html: string, insert: (html: string) => void,
        history: (operations: ProtyleOperation[], committed: Promise<NativeHistory | undefined>) => void): Promise<boolean> {
        const template = document.createElement("template");
        template.innerHTML = html;
        const images = Array.from(template.content.querySelectorAll<HTMLImageElement>("img[src]"))
            .filter(image => /^data:/i.test(image.getAttribute("src") ?? ""));
        if (!images.length) { return false; }
        const port = this.active(), epoch = this.epoch;
        this.flushPendingInput();
        if (!this.canRestoreHistory || images.length > 20) { throw new SessionError("unsaved_editor_state"); }
        let bytes = 0;
        const assets: Record<string, string> = {};
        const previews = new Map<string, string>();
        this.uploading = true;
        port.setDisabled(true);
        let changedDOM = false;
        try {
            await this.session.flush();
            this.session.markInputDirty(true);
            for (const image of images) {
                const match = /^data:image\/(png|jpeg|gif|webp);base64,([A-Za-z0-9+/]+={0,2})$/i.exec(image.getAttribute("src") ?? "");
                if (!match || match[2].length > 12 * 1024 * 1024) { throw new SessionError("invalid_clipboard_image"); }
                const content = Uint8Array.from(atob(match[2]), byte => byte.charCodeAt(0));
                bytes += content.length;
                if (bytes > 8 * 1024 * 1024) { throw new SessionError("asset_size_limit"); }
                const format = match[1].toLowerCase(), name = "clipboard-" + crypto.randomUUID() + "." + format;
                const url = this.session.createOwnedURL(new Blob([content.buffer], {type: "image/" + format}));
                assets[name] = match[2];
                previews.set(name, url);
                image.setAttribute("src", "assets/" + name);
            }
            const value = await this.session.requestUI("/api/lute/html2BlockDOM", {dom: template.innerHTML});
            if (typeof value !== "string") { throw new SessionError("invalid_ui_document"); }
            const prepared = await this.prepareAssets(value, this.document, previews);
            if (epoch !== this.epoch || this.port !== port || this.frozen) { throw new SessionError("stale_response"); }
            this.clipboardOperations = [];
            port.setDisabled(false);
            insert(prepared);
            changedDOM = true;
            const captured = this.clipboardOperations;
            this.clipboardOperations = null;
            const operations = captured.map(({action, id, data, parentID, previousID, nextID}) => ({
                action, id, parentID, previousID, nextID,
                data: typeof data === "string" && ["insert", "update", "append", "appendInsert", "prependInsert"].includes(action)
                    ? this.restoreAssetReferences(data) : data,
            }));
            const work = this.session.transact(this.document, operations, true, assets);
            this.session.markInputDirty(false);
            history(captured, work);
            await work;
            return true;
        } catch (error) {
            if (changedDOM || this.session.status !== "active") {
                this.reject(error);
            } else {
                this.session.markInputDirty(false);
                port.setDisabled(false);
            }
            throw error;
        } finally {
            this.clipboardOperations = null;
            if (!changedDOM) {
                for (const url of previews.values()) {
                    this.assets.delete(url);
                    this.session.releaseOwnedURL(url);
                }
            }
            this.uploading = false;
        }
    }
    exportHTML(markdown = false): Promise<Blob> {return this.exportSnapshot(() => this.session.exportHTML(this.document, markdown));}
    exportDOCX(): Promise<Blob> {return this.exportSnapshot(() => this.session.exportDOCX(this.document));}
    exportPDF(): Promise<Blob> {return this.exportSnapshot(() => this.session.exportPDF(this.document));}
    exportDocument(): Promise<Blob> {return this.exportSnapshot(() => this.session.exportDocument(this.document));}
    exportArchive(endpoint: string, data: unknown): Promise<unknown> {
        return this.exportSnapshot(async () => {
            await this.session.flush();
            return this.session.requestUI(endpoint, data);
        });
    }
    private async exportSnapshot<T>(read: () => Promise<T>): Promise<T> {
        const port=this.active(),epoch=this.epoch;
        this.flushPendingInput();
        if (this.restoring || this.refreshing || this.exporting || this.uploading || this.composing || this.session.unconfirmedInput) {throw new SessionError("unsaved_editor_state");}
        this.exporting=true;port.setDisabled(true);
        try {
            const result = await read();
            if(this.port!==port || epoch!==this.epoch || this.frozen || this.session.status!=="active") {throw new SessionError("stale_response");}
            return result;
        }
        finally {
            this.exporting=false;
            if (this.port===port && epoch===this.epoch && !this.frozen && this.session.status==="active") {port.setDisabled(false);}
        }
    }
    async prepareUIFragment(html: string, doc: DocumentKey): Promise<string> {
        this.active();
        const epoch = this.epoch;
        const result = await this.prepareAssets(html, doc);
        if (epoch !== this.epoch || this.frozen || this.session.status !== "active") {
            throw new SessionError("stale_response");
        }
        return result;
    }
    async prepareReadOnlyUIFragment(html: string, doc: DocumentKey): Promise<string> {
        const fragment = document.createElement("template");
        fragment.innerHTML = await this.prepareUIFragment(html, doc);
        fragment.content.querySelectorAll("[contenteditable]").forEach(node => node.setAttribute("contenteditable", "false"));
        return fragment.innerHTML;
    }
    private async prepareAssets(html: string, doc = this.document, previews?: ReadonlyMap<string, string>): Promise<string> {
        const template = document.createElement("template"); template.innerHTML = html;
        template.content.querySelectorAll("script,object,embed,link,style,template,form,base,meta,[data-wiki-preview]").forEach(element => element.remove());
        template.content.querySelectorAll("iframe").forEach(frame => {
            const block = frame.closest("[data-type=NodeIFrame],[data-type=NodeWidget]");
            if (!block) { frame.remove(); return; }
            const inert = document.createElement("span");
            inert.dataset.wikiNativeFrame = frame.outerHTML;
            inert.contentEditable = "false";
            const url = frame.getAttribute("src") || "";
            const widget = /^\/widgets\/([a-zA-Z0-9][a-zA-Z0-9_-]{0,127})\/$/.exec(url);
            if (this.widgetEnabled && widget && block.getAttribute("data-type") === "NodeWidget") {
                inert.append(externalControl("widget", widget[1]));
            } else if (externalURL(url)) {
                inert.append(externalControl("iframe", url));
            }
            frame.replaceWith(inert);
        });
        for (const element of Array.from(template.content.querySelectorAll("*"))) {
            for (const attribute of Array.from(element.attributes)) {
                if (/^on/i.test(attribute.name) || ["srcset", "srcdoc"].includes(attribute.name)) { element.removeAttribute(attribute.name); }
            }
            for (const attribute of ["src", "data-src", "poster"]) {
                const value = element.getAttribute(attribute); if (!value) { continue; }
                element.removeAttribute(attribute);
                const relative = value.startsWith("/assets/") ? value.slice(8) : value.startsWith("assets/") ? value.slice(7) : null;
                if (relative !== null) {
                    let name: string; try { name = decodeURIComponent(relative); } catch { throw new SessionError("invalid_asset_path"); }
                    const blob = previews?.get(name) ?? await this.session.assetUrl(name, doc);
                    this.assets.set(blob, value); element.setAttribute(attribute, blob);
                } else {
                    element.setAttribute("data-wiki-native-" + attribute,value);
                    if (attribute === "src" && externalURL(value)) {
                        const kind = element.localName === "img" ? "image" : element.localName;
                        if (kind === "image" || kind === "video" || kind === "audio") {
                            element.after(externalControl(kind, value));
                        }
                    }
                }
                // Remote/data/unowned local embeds stay blocked; their native
                // source URL remains in the AST and is not rewritten on disk.
            }
            const href = element.getAttribute("href");
            if (href && !/^https?:\/\//.test(href)) {
                element.setAttribute("data-wiki-native-href",href); element.removeAttribute("href");
                if (/^\/?assets\//.test(href)) { element.setAttribute("data-href",href); }
            }
            const target=element.getAttribute("data-href");
            if (target && !/^(https?:\/\/|\/?assets\/)/.test(target)) {
                element.setAttribute("data-wiki-native-data-href",target);element.removeAttribute("data-href");
            }
        }
        return template.innerHTML;
    }
    private restoreAssetReferences(html: string): string {
        const template = document.createElement("template"); template.innerHTML = html;
        template.content.querySelectorAll("[data-wiki-preview]").forEach(node => node.remove());
        template.content.querySelectorAll<HTMLElement>("[data-wiki-native-frame]").forEach(node => {
            const frame = document.createElement("template");
            frame.innerHTML = node.dataset.wikiNativeFrame ?? "";
            const element = frame.content.firstElementChild;
            if (frame.content.childElementCount === 1 && element?.localName === "iframe") {
                node.replaceWith(element);
            }
        });
        for (const element of Array.from(template.content.querySelectorAll("*"))) {
            for (const name of ["src","data-src","poster","href","data-href"]) {
                const marker="data-wiki-native-"+name;
                const native=element.getAttribute(marker);
                if (native!==null) {element.setAttribute(name,native);element.removeAttribute(marker);}
            }
            for (const attribute of Array.from(element.attributes)) {
                if (!attribute.value.includes("blob:")) { continue; }
                const native = this.assets.get(attribute.value);
                if (!native) { throw new SessionError("unowned_asset_reference"); }
                element.setAttribute(attribute.name, native);
            }
        }
        return template.innerHTML;
    }
}
