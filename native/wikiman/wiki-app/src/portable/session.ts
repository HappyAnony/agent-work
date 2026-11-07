/** Portable transport/session layer. Does not boot the legacy global editor. */
export interface EditorView {
    readonly storeId: string;
    readonly pageId: string;
    readonly sessionId: string;
    readonly mode: "wikiDesktop" | "docxSingle";
    readonly ticket: string;
    readonly viewId?: string;
    readonly entryDocument?: DocumentKey;
}
export interface DocumentKey { readonly boxId: string; readonly docId: string }
export interface DocumentVersion extends DocumentKey {
    versionId?: string;
    legacyHistoryPath?: string;
    versionHash: string;
}
export const DOC_VERSIONS_ENDPOINT = "/api/history/docVersions";
export interface NativeHistory { readonly operationId: string }
export interface AttributeCellVersion { readonly blockId: string; readonly hash: string }
export type ArchiveEndpoint = "/api/import/importSY" | "/api/import/importZipMd" | "/api/import/importStdMd";
export function isArchiveEndpoint(endpoint: string): endpoint is ArchiveEndpoint {
    return endpoint === "/api/import/importSY" || endpoint === "/api/import/importZipMd" || endpoint === "/api/import/importStdMd";
}

export interface ProtyleOperation {
    action: string; id: string; data?: string | Record<string, string | null>;
    parentID?: string; previousID?: string; nextID?: string;
}
export class SessionError extends Error {
    constructor(public readonly code: string, public readonly raw?: string) { super(code); }
}
export function recoverableUIValidation(error: unknown, guarded = false): boolean {
    return error instanceof SessionError && (guarded && error.code === "source_conflict" ||
        ["invalid_argument", "ui_endpoint_unavailable", "unauthorized", "not_found"].includes(error.code));
}

type Phase = "active" | "freezing" | "frozen" | "failed" | "stale" | "closed";
interface Options {
    fetch?: typeof fetch;
    socket?: (url: string, protocols: string[]) => WebSocket;
    dirty?: (view: EditorView, dirty: boolean) => void;
}

// Extract a raw JSON property without converting AST numbers to IEEE-754.
// JSON.parse is used only for validation/keys; its numeric values are discarded.
export function rawProperty(source: string, property: string): string {
    JSON.parse(source);
    let i = 0;
    const whitespace = () => { while (/\s/.test(source[i] ?? "!")) { i++; } };
    const valueEnd = () => {
        const start = i;
        let depth = 0, quoted = false, escaped = false;
        for (; i < source.length; i++) {
            const c = source[i];
            if (quoted) {
                if (escaped) { escaped = false; } else if (c === "\\") { escaped = true; } else if (c === '"') { quoted = false; }
            } else if (c === '"') { quoted = true; }
            else if (c === "{" || c === "[") { depth++; }
            else if (c === "}" || c === "]") { if (depth === 0) { break; } depth--; }
            else if (c === "," && depth === 0) { break; }
        }
        return source.slice(start, i).trim();
    };
    whitespace();
    if (source[i++] !== "{") { throw new SessionError("invalid_json_object"); }
    while (i < source.length) {
        whitespace();
        if (source[i] === "}") { break; }
        const start = i++;
        let escaped = false;
        while (i < source.length) {
            const c = source[i++];
            if (escaped) { escaped = false; } else if (c === "\\") { escaped = true; } else if (c === '"') { break; }
        }
        const key = JSON.parse(source.slice(start, i));
        whitespace();
        if (source[i++] !== ":") { throw new SessionError("invalid_json_object"); }
        whitespace();
        const raw = valueEnd();
        if (key === property) { return raw; }
        if (source[i] === ",") { i++; } else { break; }
    }
    throw new SessionError("missing_json_property");
}

export class EditorSession {
    readonly view: EditorView;
    private readonly base: string;
    private readonly fetcher: typeof fetch;
    private readonly options: Options;
    private phase: Phase = "active";
    private tail: Promise<void> = Promise.resolve();
    private pending = 0;
    private inputDirty = false;
    private failure: unknown = null;
    private readonly hashes = new Map<string, string>();
    private readonly histories = new WeakMap<NativeHistory, {key: string; beforeHash: string; afterHash: string}>();
    private readonly urls = new Set<string>();
    private socket: WebSocket | null = null;
    private rejectSocketReady: ((error:unknown)=>void)|null=null;
    private epoch = 0;
    private readonly ownedOperations = new Set<string>();
    private readonly eventListeners = new Set<(event: Readonly<Record<string, unknown>>) => void>();
    private readonly staleListeners = new Set<() => void>();
    constructor(baseUrl: string, view: EditorView, options: Options = {}) {
        const base = new URL(baseUrl);
        if (base.protocol !== "http:" || !["127.0.0.1", "[::1]"].includes(base.hostname) || base.username || base.password || base.pathname !== "/" || base.search || base.hash) {
            throw new SessionError("invalid_server_origin");
        }
        if (!view.storeId || !/^\d{19}$/.test(view.pageId) || !view.sessionId || !view.ticket || !["wikiDesktop", "docxSingle"].includes(view.mode)) { throw new SessionError("invalid_editor_view"); }
        this.base = base.origin;
        this.view = Object.freeze({...view, ...(view.entryDocument ? {entryDocument: Object.freeze({...view.entryDocument})} : {})});
        // Browser-native fetch checks its receiver; invoking it as a session
        // member otherwise throws Illegal invocation (Node mocks do not).
        this.fetcher = (options.fetch ?? globalThis.fetch).bind(globalThis);
        this.options = {...options};
    }
    get status(): Phase { return this.phase; }
    ownsOperation(id: string): boolean { return this.ownedOperations.has(id); }
    onStale(listener: () => void): () => void { this.staleListeners.add(listener); return () => this.staleListeners.delete(listener); }
    private own(id: string): void { this.ownedOperations.add(id); if (this.ownedOperations.size > 1024) { this.ownedOperations.delete(this.ownedOperations.values().next().value!); } }
    get dirty(): boolean { return this.inputDirty || this.pending > 0 || this.failure !== null; }
    get unconfirmedInput(): boolean { return this.inputDirty; }
    markInputDirty(dirty: boolean): void { if (dirty) { this.active(); } this.inputDirty = dirty; this.notify(); }
    fail(error: unknown): void { this.failure = error; this.phase = "failed"; this.notify(); }
    private key(doc: DocumentKey): string { return `${doc.boxId}/${doc.docId}`; }
    private headers(): Record<string, string> { return {"X-Wiki-Editor": this.view.ticket, "X-Wiki-Session": this.view.sessionId}; }
    private active(): void { if (this.phase !== "active") { throw new SessionError("editor_not_active"); } }
    private notify(): void {
        try { this.options.dirty?.(this.view, this.dirty); }
        catch (error) { this.failure = error; this.phase = "failed"; }
    }
    private async fetchBytes(url: string, init: RequestInit): Promise<{response: Response; bytes: Uint8Array}> {
        const controller = new AbortController();
        const timer = setTimeout(() => controller.abort(), 30_000);
        try {
            const response = await this.fetcher(url, {...init, signal: controller.signal});
            return {response, bytes: await limitedBytes(response)};
        } finally { clearTimeout(timer); }
    }
    private async request(fields: Record<string, unknown>, rawAst?: string): Promise<string> {
        const payload = {...fields, protocolVersion: 1, storeId: this.view.storeId, pageId: this.view.pageId};
        // Never JSON.stringify a parsed AST. Pass its original lexical JSON.
        const prefix = JSON.stringify(payload);
        const body = rawAst === undefined ? prefix : prefix.slice(0, -1) + ',"ast":' + rawAst + "}";
        if (new TextEncoder().encode(body).length > 16 * 1024 * 1024) { throw new SessionError("request_limit"); }
        const {response, bytes} = await this.fetchBytes(this.base + "/v1/content", {method: "POST", headers: {...this.headers(), "Content-Type": "application/json"}, body, credentials: "omit", redirect: "error", cache: "no-store"});
        const raw = new TextDecoder("utf-8", {fatal: true}).decode(bytes);
        const envelope = JSON.parse(raw);
        if (envelope.protocolVersion !== 1) { throw new SessionError("protocol_mismatch", raw); }
        if (!response.ok || !envelope.ok) {
            const code = envelope.error?.type ?? "request_failed";
            if (["unauthorized", "stale_session", "source_identity_changed"].includes(code)) { this.invalidate(); }
            throw new SessionError(code, raw);
        }
        if (envelope.result?.storeId !== this.view.storeId || envelope.result?.pageId !== this.view.pageId) { this.invalidate(); throw new SessionError("response_identity_mismatch"); }
        return raw;
    }
    /** An original component provider consumes only its fixed UI endpoint. */
    requestUI(endpoint: string, data: unknown = {}): Promise<unknown> {
        this.active();
        return this.exchangeUI(endpoint, data, false);
    }
    async pluginStorage(data: {plugin: string; file: string; action: string; value?: string}): Promise<unknown> {
        this.active();
        const endpoint = "/api/plugin/sandboxedStorage";
        if (data.action === "load") {
            return this.exchangeUI(endpoint, data, false);
        }
        const operationId = crypto.randomUUID();
        this.own(operationId);
        let result: unknown;
        await this.enqueue(async () => { result = await this.exchangeUI(endpoint, data, true, operationId); }, true);
        return result;
    }
    private async exchangeUI(endpoint: string, data: unknown, queued: boolean, operationId?: string): Promise<unknown> {
        const epoch = this.epoch;
        const body = JSON.stringify({protocolVersion: 1, storeId: this.view.storeId, pageId: this.view.pageId, endpoint, data, operationId});
        const limit = isArchiveEndpoint(endpoint) ? 12 : 1;
        if (new TextEncoder().encode(body).length > limit * 1024 * 1024) { throw new SessionError("request_limit"); }
        const {response, bytes} = await this.fetchBytes(this.base + "/v1/ui", {
            method: "POST", headers: {...this.headers(), "Content-Type": "application/json"},
            body, credentials: "omit", redirect: "error", cache: "no-store",
        });
        if (epoch !== this.epoch || (this.phase !== "active" && !(queued && this.phase === "freezing"))) {
            throw new SessionError("stale_response");
        }
        const envelope = JSON.parse(new TextDecoder("utf-8", {fatal: true}).decode(bytes));
        if (envelope.protocolVersion !== 1) { throw new SessionError("protocol_mismatch"); }
        if (!response.ok || !envelope.ok) {
            // A denied UI action is a provider error. Only the server's
            // explicit stale-session response retires the editor.
            const code = envelope.error?.type ?? "ui_request_failed";
            if (code === "stale_session") { this.invalidate(); }
            throw new SessionError(code);
        }
        const value = envelope.result;
        if (value?.storeId !== this.view.storeId || value.pageId !== this.view.pageId ||
            value.sessionId !== this.view.sessionId || !this.view.viewId || value.viewId !== this.view.viewId) {
            this.invalidate();
            throw new SessionError("response_identity_mismatch");
        }
        return value.data;
    }
    async readDocument(doc: DocumentKey): Promise<{ast: string; hash: string}> {
        this.active();
        const epoch = this.epoch;
        const raw = await this.request({action: "document.get", ...doc});
        if (epoch !== this.epoch || this.phase !== "active") { throw new SessionError("stale_response"); }
        const result = rawProperty(raw, "result");
        const hash = JSON.parse(rawProperty(result, "hash")) as string;
        if (!/^[a-f0-9]{64}$/.test(hash)) { throw new SessionError("invalid_content_hash"); }
        if (this.pending) { throw new SessionError("save_in_progress"); }
        this.hashes.set(this.key(doc), hash);
        return {ast: rawProperty(result, "ast"), hash};
    }
    async renderDocument(doc: DocumentKey): Promise<{html: string; hash: string; path?: string}> {
        this.active(); const epoch = this.epoch;
        if (this.dirty) { throw new SessionError("unsaved_editor_state"); }
        const raw = await this.request({action: "document.render", ...doc});
        if (epoch !== this.epoch || this.phase !== "active" || this.dirty) { throw new SessionError("stale_response"); }
        const result = JSON.parse(raw).result;
        if (typeof result.html !== "string" || !/^[a-f0-9]{64}$/.test(result.hash)) { throw new SessionError("invalid_render_response"); }
        let path: string | undefined;
        if (result.path !== undefined) {
            if (typeof result.path !== "string" || !result.path.startsWith(doc.boxId + "/")) { throw new SessionError("invalid_render_response"); }
            path = (result.path as string).slice(doc.boxId.length);
            if (!/^(\/\d{14}-[a-z0-9]{7})+\.docj$/.test(path) || !path.endsWith("/" + doc.docId + ".docj")) {
                throw new SessionError("invalid_render_response");
            }
        }
        this.hashes.set(this.key(doc), result.hash);
        return {html: result.html, hash: result.hash, ...(path ? {path} : {})};
    }
    private enqueue(work: () => Promise<void>, recoverValidation = false, guarded = false): Promise<void> {
        this.pending++; this.notify();
        const queued = this.tail.then(async () => {
            if (this.failure !== null || !["active", "freezing"].includes(this.phase)) { throw new SessionError("save_queue_blocked"); }
            await work();
        });
        const settled = queued.catch(error => {
            if (!recoverValidation || !recoverableUIValidation(error, guarded)) {
                this.failure = error;
                if (this.phase !== "stale" && this.phase !== "closed") { this.phase = "failed"; }
            }
            throw error;
        }).finally(() => { this.pending--; this.notify(); });
        this.tail = settled.catch(() => undefined); return settled;
    }
    transact(doc: DocumentKey, operations: readonly ProtyleOperation[], captureHistory = false,
        clipboardAssets?: Record<string, string>): Promise<NativeHistory | undefined> {
        this.active();
        if (!this.hashes.has(this.key(doc))) { throw new SessionError("read_before_write_required"); }
        if (!operations.length || operations.length > 1000) { throw new SessionError("invalid_operations"); }
        const captured = JSON.parse(JSON.stringify(operations));
        const target = Object.freeze({...doc}); const operationId = crypto.randomUUID();
        this.own(operationId);
        let history: NativeHistory | undefined;
        return this.enqueue(async () => {
            const expectedHash = this.hashes.get(this.key(target))!;
            const raw = await this.request({action: "document.transactions", ...target, operationId, expectedHash, operations: captured,
                ...(captureHistory ? {nativeHistory: true} : {}), clipboardAssets});
            const result = JSON.parse(raw).result;
            if (!result.receipt?.sourceCommitted) { throw new SessionError("source_not_committed", raw); }
            if (result.originalIndexFresh !== true) { throw new SessionError("index_stale", raw); }
            if (!/^[a-f0-9]{64}$/.test(result.hash)) { throw new SessionError("committed_version_unavailable", raw); }
            if (captureHistory) {
                if (result.historyId !== operationId || result.receipt.operationId !== operationId) { throw new SessionError("invalid_native_history", raw); }
                history = Object.freeze({operationId});
                this.histories.set(history, {key: this.key(target), beforeHash: expectedHash, afterHash: result.hash});
            }
            this.hashes.set(this.key(target), result.hash);
        }).then(() => history);
    }
    transactOriginal(doc: DocumentKey, batches: readonly (readonly ProtyleOperation[])[], recoverValidation = false,
        cellVersion?: AttributeCellVersion): Promise<{
        history: NativeHistory;
        transactions: unknown;
    }> {
        this.active();
        if (this.view.mode !== "wikiDesktop") { throw new SessionError("richtext_operation_unsupported"); }
        if (!this.hashes.has(this.key(doc))) { throw new SessionError("read_before_write_required"); }
        if (!batches.length || batches.some(ops => !ops.length) || batches.reduce((n, ops) => n + ops.length, 0) > 1000) { throw new SessionError("invalid_operations"); }
        const captured = JSON.parse(JSON.stringify(batches));
        const target = Object.freeze({...doc});
        const operationId = crypto.randomUUID();
        this.own(operationId);
        const guard = cellVersion && {...cellVersion};
        let acknowledged: {history: NativeHistory; transactions: unknown};
        return this.enqueue(async () => {
            const expectedHash = this.hashes.get(this.key(target))!;
            const value = await this.exchangeUI("/api/transactions", {
                ...target, expectedHash, operationId, cellVersion:guard,
                transactions: captured.map((doOperations: ProtyleOperation[]) => ({doOperations})),
            }, true) as {hash: string; receipt?: {sourceCommitted?: boolean; operationId?: string};
                indexFresh?: boolean; historyId?: string; transactions: unknown};
            if (!value.receipt?.sourceCommitted || value.receipt.operationId !== operationId) {
                throw new SessionError("source_not_committed");
            }
            if (!value.indexFresh) { throw new SessionError("index_stale"); }
            if (value.historyId !== operationId || !/^[a-f0-9]{64}$/.test(value.hash)) {
                throw new SessionError("invalid_native_history");
            }
            const history = Object.freeze({operationId});
            this.histories.set(history, {key: this.key(target), beforeHash: expectedHash, afterHash: value.hash});
            this.hashes.set(this.key(target), value.hash);
            acknowledged = {history, transactions: value.transactions};
        }, recoverValidation, !!guard).then(() => acknowledged);
    }
    restoreHistory(doc: DocumentKey, history: NativeHistory, redo: boolean): Promise<void> {
        this.active();
        const owned = this.histories.get(history);
        if (!owned || owned.key !== this.key(doc)) { throw new SessionError("foreign_native_history"); }
        const target = Object.freeze({...doc});
        const expectedHash = redo ? owned.beforeHash : owned.afterHash;
        const restoredHash = redo ? owned.afterHash : owned.beforeHash;
        const operationId = crypto.randomUUID();
        this.own(operationId);
        return this.enqueue(async () => {
            if (this.hashes.get(this.key(target)) !== expectedHash) { throw new SessionError("history_version_conflict"); }
            const raw = await this.request({action: "document.history", ...target, historyId: history.operationId, redo, operationId, expectedHash});
            const result = JSON.parse(raw).result;
            if (!result.receipt?.sourceCommitted) { throw new SessionError("source_not_committed", raw); }
            if (result.originalIndexFresh !== true) { throw new SessionError("index_stale", raw); }
            if (result.receipt.operationId !== operationId || result.hash !== restoredHash) { throw new SessionError("invalid_native_history", raw); }
            this.hashes.set(this.key(target), restoredHash);
        });
    }
    private async digest(ast: string): Promise<string> {
        const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(ast));
        return Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, "0")).join("");
    }
    async flush(): Promise<void> {
        await this.tail;
        if (this.dirty) { throw new SessionError("unsaved_editor_state"); }
    }
    saveDocument(doc: DocumentKey, ast: string): Promise<void> {
        return this.writeDocument(doc, ast);
    }
    private writeDocument(doc: DocumentKey, ast: string): Promise<void> {
        this.active();
        JSON.parse(ast); // Syntax check only, preserving original lexical numbers.
        if (!this.hashes.has(this.key(doc))) { throw new SessionError("read_before_write_required"); }
        const target = Object.freeze({...doc});
        const nativeAst = ast.trim();
        const operationId = crypto.randomUUID();
        this.own(operationId);
        return this.enqueue(async () => {
            // The core stores document.put's exact raw AST bytes. Derive OUR
            // committed hash, never adopt a later writer's hash by re-reading:
            // doing so could let a queued old snapshot overwrite that writer.
            const committedHash = await this.digest(nativeAst);
            const fields = {...target, operationId, expectedHash: this.hashes.get(this.key(target))};
            const raw = await this.request({action: "document.put", ...fields}, nativeAst);
            const result = JSON.parse(raw).result;
            if (!result.receipt?.sourceCommitted) { throw new SessionError("source_not_committed", raw); }
            this.hashes.set(this.key(target), committedHash);
        });
    }
    /** Drain native transaction/raw-write queues. Undo/redo belongs to the
     * same session and must never replay pre-resume history into a new one. */
    async prepareRelease(): Promise<void> {
        if (this.phase === "active") { this.phase = "freezing"; this.epoch++; }
        if (!["freezing", "frozen", "failed"].includes(this.phase)) { throw new SessionError("editor_not_active"); }
        await this.tail;
        if (this.dirty) { throw new SessionError("unsaved_editor_state"); }
        this.phase = "frozen"; this.releaseResources();
    }
    private async encodeLocalFile(file: Blob): Promise<string> {
        const bytes = new Uint8Array(await file.arrayBuffer());
        if (bytes.length > 8 * 1024 * 1024) { throw new SessionError("asset_size_limit"); }
        let binary = "";
        for (let offset = 0; offset < bytes.length; offset += 32768) {
            binary += String.fromCharCode(...bytes.subarray(offset, offset + 32768));
        }
        return btoa(binary);
    }
    async importArchive(endpoint: ArchiveEndpoint, form: FormData): Promise<void> {
        this.active();
        const selected = form.getAll("file");
        const files = selected.filter((file): file is File => file instanceof File);
        const notebook = form.get("notebook"), toPath = form.get("toPath");
        const local = endpoint === "/api/import/importStdMd";
        if (this.view.mode !== "wikiDesktop" || typeof notebook !== "string" || typeof toPath !== "string" ||
            !files.length || files.length > (local ? 1000 : 1) || selected.length !== files.length ||
            [...form.keys()].some(key => !["file", "notebook", "toPath"].includes(key)) ||
            form.getAll("notebook").length !== 1 || form.getAll("toPath").length !== 1 ||
            files.reduce((size, file) => size + file.size, 0) > 8 * 1024 * 1024) {
            throw new SessionError("invalid_ui_archive_request");
        }
        const operationId = crypto.randomUUID();
        this.own(operationId);
        await this.enqueue(async () => {
            if (local) {
                const entries: Record<string, string> = Object.create(null);
                for (const file of files) {
                    const name = file.webkitRelativePath || file.name;
                    if (Object.prototype.hasOwnProperty.call(entries, name)) { throw new SessionError("invalid_ui_archive_request"); }
                    entries[name] = await this.encodeLocalFile(file);
                }
                await this.exchangeUI(endpoint, {notebook, toPath, files: entries}, true, operationId);
            } else {
                const file = files[0];
                const data = await this.encodeLocalFile(file);
                const name = endpoint === "/api/import/importZipMd" ? file.name : undefined;
                await this.exchangeUI(endpoint, {notebook, toPath, data, name}, true, operationId);
            }
        }, true);
    }
    async renameDocument(doc: DocumentKey, data: {notebook: string; path: string; title: string}): Promise<void> {
        this.active();
        const expectedHash = this.hashes.get(this.key(doc));
        if (!expectedHash || data.notebook !== doc.boxId || !data.path.endsWith("/" + doc.docId + ".docj")) {
            throw new SessionError("invalid_ui_title_request");
        }
        const operationId = crypto.randomUUID();
        this.own(operationId);
        await this.enqueue(async () => {
            await this.exchangeUI("/api/filetree/renameDoc", {...data, expectedHash}, true, operationId);
        }, true);
    }
    async createDocumentReference(doc: DocumentKey, data: {
        id: string; notebook: string; title: string; hPath?: string; parentId?: string; blockId: string; blockDOM: string;
    }): Promise<void> {
        this.active();
        if (this.view.mode !== "wikiDesktop" || !this.hashes.has(this.key(doc))) {
            throw new SessionError("read_before_write_required");
        }
        const operationId = crypto.randomUUID();
        this.own(operationId);
        await this.enqueue(async () => {
            const expectedHash = this.hashes.get(this.key(doc));
            const result = await this.exchangeUI("/api/filetree/createDocWithReference", {
                ...doc, ...data, expectedHash,
            }, true, operationId) as {id: string; receipt?: {operationId: string; sourceCommitted: boolean}; indexFresh: boolean};
            if (result.id !== data.id || !result.receipt?.sourceCommitted || result.receipt.operationId !== operationId) {
                throw new SessionError("source_not_committed");
            }
            if (!result.indexFresh) { throw new SessionError("index_stale"); }
        }, true, true);
    }
    async renameTag(oldLabel: string, newLabel: string): Promise<void> {
        this.active();
        const operationId = crypto.randomUUID();
        this.own(operationId);
        await this.enqueue(async () => {
            await this.exchangeUI("/api/tag/renameTag", {oldLabel, newLabel}, true, operationId);
        }, true);
    }
    async restoreDocumentVersion(version: DocumentVersion & {expectedHash: string}): Promise<void> {
        this.active();
        const current = this.hashes.get(this.key(version));
        if (current && current !== version.expectedHash) { throw new SessionError("stale_version_preview"); }
        const operationId = crypto.randomUUID();
        this.own(operationId);
        await this.enqueue(async () => {
            const result = await this.exchangeUI(DOC_VERSIONS_ENDPOINT, {...version, action:"restore"}, true, operationId) as {
                receipt?: {sourceCommitted?: boolean; operationId?: string};
                originalIndexFresh?: boolean;
                hash?: string;
            };
            if (result.receipt?.sourceCommitted !== true || result.receipt.operationId !== operationId ||
                result.originalIndexFresh !== true || result.hash !== version.versionHash) {
                throw new SessionError("invalid_native_history");
            }
        }, true);
    }
    /** Create-only, page-scoped upload; bytes are captured inside the owning
     * save queue, including File.arrayBuffer() (not just the HTTP request). */
    async putAsset(file: Blob, filename: string): Promise<string> {
        this.active();
        if (file.size > 8 * 1024 * 1024) { throw new SessionError("asset_size_limit"); }
        const extension = /\.([a-zA-Z0-9]{1,12})$/.exec(filename)?.[1].toLowerCase();
        const name = crypto.randomUUID() + (extension ? "." + extension : "");
        const operationId = crypto.randomUUID(); this.own(operationId);
        await this.enqueue(async () => {
            const data = await this.encodeLocalFile(file);
            const raw = await this.request({action:"asset.put",assetName:name,data,operationId,expectedHash:""});
            if (!JSON.parse(raw).result.receipt?.sourceCommitted) { throw new SessionError("source_not_committed",raw); }
        });
        return name;
    }
    async assetUrl(name: string, doc?: DocumentKey): Promise<string> {
        this.active(); const epoch = this.epoch;
        const url = new URL(this.base + "/v1/resource");
        url.search = new URLSearchParams({storeId: this.view.storeId, pageId: this.view.pageId, name, ...(doc ? {boxId: doc.boxId, docId: doc.docId} : {})}).toString();
        const {response, bytes: data} = await this.fetchBytes(url.toString(), {headers: this.headers(), credentials: "omit", redirect: "error", cache: "no-store"});
        if (!response.ok) { throw new SessionError("asset_read_failed"); }
        if (epoch !== this.epoch || this.phase !== "active") { throw new SessionError("stale_response"); }
        // Never trust an uploaded HTML/SVG MIME type as active browser content.
        const types: Record<string,string> = {png:"image/png",jpg:"image/jpeg",jpeg:"image/jpeg",gif:"image/gif",webp:"image/webp",avif:"image/avif",mp3:"audio/mpeg",ogg:"audio/ogg",wav:"audio/wav",mp4:"video/mp4",webm:"video/webm",pdf:"application/pdf"};
        const type = types[name.split(".").pop()?.toLowerCase() ?? ""] ?? "application/octet-stream";
        return this.createOwnedURL(new Blob([data.slice().buffer], {type}));
    }
    createOwnedURL(data: Blob): string {
        this.active();
        const blob = URL.createObjectURL(data);
        this.urls.add(blob); return blob;
    }
    releaseOwnedURL(blob: string): void {
        if (this.urls.delete(blob)) { URL.revokeObjectURL(blob); }
    }
    async exportDocument(doc: DocumentKey): Promise<Blob> {
        const target=Object.freeze({...doc});
        await this.flush();
        const document = await this.readDocument(target);
        return new Blob([document.ast], {type: "application/json"});
    }
    async exportHTML(doc: DocumentKey, markdown = false): Promise<Blob> {
        this.active(); const epoch=this.epoch;
        const target=Object.freeze({...doc});
        await this.flush();
        if(epoch!==this.epoch || this.phase!=="active") {throw new SessionError("stale_response");}
        const raw=await this.request({action:"document.export",...target,format:markdown ? "htmlmd" : undefined});
        if (epoch!==this.epoch || this.phase!=="active" || this.dirty) {throw new SessionError("stale_response");}
        const result=JSON.parse(raw).result;
        if (typeof result.html!=="string" || !/^[a-f0-9]{64}$/.test(result.hash)) {throw new SessionError("invalid_export_response");}
        // Read-only export must never advance the save queue's CAS baseline.
        return new Blob([result.html],{type:"text/html;charset=utf-8"});
    }
    async exportDOCX(doc: DocumentKey): Promise<Blob> {
        return this.exportBinary(doc,"docx");
    }
    async exportPDF(doc: DocumentKey): Promise<Blob> {
        return this.exportBinary(doc,"pdf");
    }
    private async exportBinary(doc: DocumentKey, format:"docx"|"pdf"): Promise<Blob> {
        this.active(); const epoch=this.epoch, target=Object.freeze({...doc});
        await this.flush();
        if(epoch!==this.epoch || this.phase!=="active") {throw new SessionError("stale_response");}
        const raw=await this.request({action:"document.export",format,...target});
        if(epoch!==this.epoch || this.phase!=="active" || this.dirty) {throw new SessionError("stale_response");}
        const result=JSON.parse(raw).result;
        const mime=format==="pdf"?"application/pdf":"application/vnd.openxmlformats-officedocument.wordprocessingml.document";
        if(result.mediaType!==mime || typeof result.data!=="string" || result.data.length>Math.ceil(16*1024*1024/3)*4 || !/^[a-f0-9]{64}$/.test(result.hash)) {throw new SessionError("invalid_export_response");}
        const bytes=Uint8Array.from(atob(result.data),c=>c.charCodeAt(0));
        const signature=format==="pdf"?new TextDecoder().decode(bytes.subarray(0,5))==="%PDF-":bytes[0]===80&&bytes[1]===75;
        if(bytes.length>16*1024*1024 || !signature) {throw new SessionError("invalid_export_response");}
        // An export never updates this editor's save CAS version.
        return new Blob([bytes.buffer],{type:mime});
    }
    onWorkspaceEvent(listener: (event: Readonly<Record<string, unknown>>) => void): () => void {
        this.active();
        this.eventListeners.add(listener);
        return () => this.eventListeners.delete(listener);
    }
    subscribe(onEvent: (event: Readonly<Record<string, unknown>>, view: EditorView) => void): Promise<void> {
        this.active();
        if (this.socket) { throw new SessionError("already_subscribed"); }
        const url = new URL(this.base + "/v1/events"); url.protocol = "ws:";
        url.search = new URLSearchParams({storeId: this.view.storeId, pageId: this.view.pageId}).toString();
        const epoch = this.epoch;
        const socket = (this.options.socket ?? ((url, protocols) => new WebSocket(url, protocols)))(url.toString(), ["wiki.v1", "wiki.editor." + this.view.ticket, "wiki.session." + this.view.sessionId]);
        this.socket = socket;
        let resolveReady!:()=>void;
        const ready=new Promise<void>((resolve,reject)=>{resolveReady=resolve;this.rejectSocketReady=reject;});
        const timer=setTimeout(()=>this.invalidate(),10000);
        const settled=ready.finally(()=>clearTimeout(timer));
        socket.onmessage = (message) => {
            if (epoch !== this.epoch || this.phase !== "active") { return; }
            try {
                const event = JSON.parse(String(message.data));
                if (event.storeId !== this.view.storeId || event.pageId !== this.view.pageId ||
                    this.view.viewId && (event.viewId !== this.view.viewId || event.sessionId !== this.view.sessionId)) {
                    this.invalidate(); return;
                }
                if(event.type==="workspaceReady") {this.rejectSocketReady=null;resolveReady();return;}
                const payload = Object.freeze(event);
                for (const listener of this.eventListeners) {
                    try { listener(payload); } catch { /* A component notification is advisory. */ }
                }
                if (event.type === "resyncRequired") { this.invalidate(); return; }
                onEvent(payload, this.view);
            } catch { this.invalidate(); }
        };
        socket.onclose = () => { if (this.phase === "active") { this.invalidate(); } };
        socket.onerror = () => { this.invalidate(); };
        return settled;
    }
    invalidate(): void { this.phase = "stale"; this.epoch++; this.releaseResources(); for (const listener of this.staleListeners) { listener(); } }
    close(): void {
        if (this.dirty) { throw new SessionError("unsaved_editor_state"); }
        this.phase = "closed"; this.epoch++; this.releaseResources();
    }
    private releaseResources(): void {
        this.eventListeners.clear();
        this.rejectSocketReady?.(new SessionError("editor_not_active"));this.rejectSocketReady=null;
        if (this.socket) { const socket = this.socket; this.socket = null; socket.onclose = null; socket.onerror = null; socket.close(); }
        for (const url of this.urls) { URL.revokeObjectURL(url); }
        this.urls.clear();
    }
}

export class EditorSessionGroup {
    constructor(private readonly sessions: readonly EditorSession[]) {}
    async prepareRelease(): Promise<void> {
        // Freeze all before awaiting any, then await every queue even on failure.
        const results = await Promise.allSettled(this.sessions.map(session => session.prepareRelease()));
        if (results.some(result => result.status === "rejected")) { throw new SessionError("unsaved_editor_state"); }
    }
    invalidate(): void { for (const session of this.sessions) { session.invalidate(); } }
}

async function limitedBytes(response: Response): Promise<Uint8Array> {
    if (!response.body) { throw new SessionError("empty_response"); }
    const reader = response.body.getReader(); const parts: Uint8Array[] = []; let size = 0;
    try {
        while (true) { const part = await reader.read(); if (part.done) { break; } size += part.value.byteLength; if (size > 32 * 1024 * 1024) { throw new SessionError("response_limit"); } parts.push(part.value); }
    } finally { await reader.cancel(); reader.releaseLock(); }
    const output = new Uint8Array(size); let offset = 0;
    for (const part of parts) { output.set(part, offset); offset += part.byteLength; }
    return output;
}
