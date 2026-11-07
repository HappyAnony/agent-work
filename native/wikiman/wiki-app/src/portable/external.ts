export const externalSlash = new Map<string, ExternalKind>([
    ["insertIframeURL", "iframe"], ["insertImgURL", "image"],
    ["insertVideoURL", "video"], ["insertAudioURL", "audio"],
]);
export type ExternalKind = "iframe" | "image" | "video" | "audio";

export function widgetPath(name: string) {
    if (/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$/.test(name)) { return "/widgets/" + name + "/"; }
}

export function externalURL(raw: string) {
    if (raw.length > 4096 || /[\s\\]/.test(raw) || !/^https?:\/\//i.test(raw)) { return; }
    try {
        const url = new URL(raw);
        if (url.hostname && !url.username && !url.password) { return url; }
    } catch { /* invalid URL */ }
}

export function externalControl(kind: ExternalKind | "widget", raw: string) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "b3-button b3-button--outline";
    button.contentEditable = "false";
    button.dataset.wikiPreview = kind;
    button.dataset.wikiPreviewUrl = raw;
    button.textContent = (window as Window & {siyuan?: {languages?: {preview?: string}}}).siyuan?.languages?.preview ?? "Preview";
    button.title = raw;
    return button;
}
