import {isPortableWindow, markPortableWindow} from "./boundary";
import {SessionError} from "./session";

type DocumentExportFormat = "html" | "htmlmd" | "docx" | "pdf";

interface ScopedProviders {
    post(endpoint: string, data: unknown): Promise<unknown>;
    get(url: string): Promise<unknown>;
    error(error: unknown): void;
    subscribe(listener: (event: Readonly<Record<string, unknown>>) => void): () => void;
    exportDocument?(docId: string, format: DocumentExportFormat): Promise<Blob>;
    assetURL?(path: string): Promise<string>;
    openDocument?(id: string): Promise<void>;
}

export async function scopedUIOpenDocument(id: string): Promise<void> {
    const current = owner;
    if (!current?.openDocument || !/^\d{14}-[a-z0-9]{7}$/.test(id)) {
        throw new SessionError("ui_document_unavailable");
    }
    try {
        await current.openDocument(id);
    } catch (error) {
        if (owner === current) { current.error(error); }
    }
}

export async function scopedUIExport(docId: string, format: DocumentExportFormat): Promise<Blob> {
    const current = owner;
    if (!current?.exportDocument) { throw new SessionError("ui_provider_closed"); }
    try {
        const blob = await current.exportDocument(docId, format);
        if (owner !== current) { throw new SessionError("stale_response"); }
        return blob;
    } catch (error) {
        if (owner === current) { current.error(error); }
        throw error;
    }
}

export async function scopedUIAssetURL(path: string): Promise<string> {
    const current = owner;
    if (!current?.assetURL) { throw new SessionError("ui_provider_closed"); }
    try {
        const url = await current.assetURL(path);
        if (owner !== current) { throw new SessionError("stale_response"); }
        return url;
    } catch (error) {
        if (owner === current) { current.error(error); }
        throw error;
    }
}

let owner: ScopedProviders | undefined;
let bound = false;

export function installScopedProviders(providers: ScopedProviders): () => void {
    markPortableWindow();
    if (bound) { throw new SessionError("ui_provider_already_attached"); }
    bound = true;
    owner = providers;
    return () => { if (owner === providers) { owner = undefined; latest.clear(); } };
}

const orderedEndpoints = new Set([
    "/api/search/searchRefBlock", "/api/graph/getGraph", "/api/graph/getLocalGraph",
    "/api/block/getRecentUpdatedBlocks", "/api/search/fullTextSearchBlock",
]);
const latest = new Map<string, number>();
let sequence = 0;

export async function scopedUIRequest(method: "post" | "get", endpoint: string, data?: unknown): Promise<unknown> {
    const current = owner;
    if (!isPortableWindow() || !current) { throw new SessionError("ui_provider_closed"); }
    const ordered = method === "post" && orderedEndpoints.has(endpoint);
    const requestId = ++sequence;
    if (ordered) { latest.set(endpoint, requestId); }
    const payload = ordered ? {...(data as Record<string, unknown>), reqId: requestId} : data;
    try {
        const value = await current[method](endpoint, payload);
        if (owner !== current || ordered && latest.get(endpoint) !== requestId) {
            throw new SessionError("stale_response");
        }
        return value;
    } catch (error) {
        if (owner === current && (!ordered || latest.get(endpoint) === requestId) &&
            !(error instanceof SessionError && (error.code === "stale_response" ||
                endpoint === "/api/asset/getFileAnnotation" && error.code === "not_found"))) {
            current.error(error);
        }
        throw error;
    }
}

export function subscribeScopedUI(listener: (event: Readonly<Record<string, unknown>>) => void): () => void {
    const current = owner;
    if (!current) { throw new SessionError("ui_provider_closed"); }
    return current.subscribe(event => { if (owner === current) { listener(event); } });
}
