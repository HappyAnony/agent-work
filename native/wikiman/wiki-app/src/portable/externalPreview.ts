import {Dialog} from "../dialog";
import {escapeHtml} from "../util/escape";
import type {ProtyleBinding} from "./protyle";
import type {ExternalKind} from "./external";

const previewTags = new Set("p div span h1 h2 h3 h4 h5 h6 pre code blockquote ul ol li table thead tbody tr td th strong b em i br hr".split(" "));
const dropTags = new Set("script style link meta iframe object embed template svg form noscript audio video img".split(" "));
type FrameForm = {action?: string; fields?: string[]; hiddenFields?: string[]; files?: Record<string, string>};
type FrameAsset = {name: string; type: string; data: string};

function inertPreview(content: HTMLElement, raw: string, plain: boolean) {
    if (plain) { content.textContent = raw; return; }
    const input = document.createElement("template");
    input.innerHTML = raw;
    const copy = (source: Node, target: Node) => {
        for (const node of source.childNodes) {
            if (node.nodeType === Node.TEXT_NODE) { target.appendChild(document.createTextNode(node.textContent)); }
            if (!(node instanceof Element) || dropTags.has(node.localName)) { continue; }
            if (previewTags.has(node.localName)) {
                const clean = document.createElement(node.localName);
                target.appendChild(clean);
                copy(node, clean);
            } else {
                copy(node, target);
            }
        }
    };
    copy(input.content, content);
}

export async function showExternalPreview(binding: ProtyleBinding, kind: ExternalKind | "widget", url: string, owner: HTMLElement) {
    let previewURL: string;
    let widgetReady: (event: MessageEvent) => void;
    let frameSubmit: (event: MessageEvent) => void;
    const dialog = new Dialog({
        title: escapeHtml(window.siyuan.languages.preview),
        content: '<div class="b3-dialog__content b3-typography" role="status"></div>',
        width: "min(720px, 85vw)",
        destroyCallback: () => {
            if (previewURL) { binding.session.releaseOwnedURL(previewURL); }
            if (widgetReady) { window.removeEventListener("message", widgetReady); }
            if (frameSubmit) { window.removeEventListener("message", frameSubmit); }
        },
    });
    const body = dialog.element.querySelector<HTMLElement>(".b3-dialog__content");
    body.textContent = window.siyuan.languages.loading;
    const current = () => dialog.element.isConnected && owner.isConnected && binding.session.status === "active";
    try {
        const result = await binding.session.requestUI(kind === "widget" ? "/api/widget/preview" : "/api/asset/previewExternal",
            kind === "widget" ? {name: url} : {kind, url}) as {type: string; text?: string; data?: string;
                interactive?: boolean; scriptBase?: string; assets?: FrameAsset[]; form?: FrameForm};
        if (!current()) { return; }
        body.replaceChildren();
        if (kind === "widget" || kind === "iframe" && result.interactive) {
            if (typeof result.text !== "string" || result.type !== (kind === "widget" ? "widget" : "text/html")) {
                throw new Error("invalid_sandbox_preview");
            }
            const frame = document.createElement("iframe");
            frame.title = url;
            frame.sandbox.add("allow-scripts");
            frame.referrerPolicy = "no-referrer";
            frame.style.cssText = "width:100%;min-height:16rem;border:0";
            const form = kind === "iframe" && binding.session.view.mode === "wikiDesktop" ? result.form : undefined;
            if (form?.action && (form.fields?.length || Object.keys(form.files || {}).length)) {
                const names = form.fields || [];
                const review = document.createElement("section");
                let submitting = false;
                frameSubmit = event => {
                    if (event.source !== frame.contentWindow || event.origin !== "null" || !current() || submitting ||
                        event.data?.type !== "wiki-frame-form" || !event.data.fields || typeof event.data.fields !== "object") { return; }
                    const fields: Record<string, string> = Object.create(null);
                    for (const key of names) {
                        const value = event.data.fields[key];
                        if (typeof value !== "string" || new TextEncoder().encode(value).length > 2048) { return; }
                        fields[key] = value;
                    }
                    const details = document.createElement("p");
                    details.textContent = "发送到 " + form.action;
                    const values = document.createElement("pre");
                    values.textContent = names.map(key => key + " = " + (form.hiddenFields?.includes(key) ?
                        "（隐藏字段）" : JSON.stringify(fields[key]))).join("\n");
                    const pickers = Object.entries(form.files || {}).map(([key, kind]) => {
                        const label = document.createElement("label");
                        const picker = document.createElement("input");
                        picker.type = "file";
                        picker.accept = kind;
                        label.textContent = key + " ";
                        label.append(picker);
                        return {key, label, picker};
                    });
                    const warning = document.createElement("p");
                    warning.setAttribute("role", "alert");
                    const confirm = document.createElement("button");
                    confirm.type = "button";
                    confirm.className = "b3-button";
                    confirm.textContent = "发送表单";
                    const cancel = document.createElement("button");
                    cancel.type = "button";
                    cancel.textContent = "取消发送";
                    cancel.addEventListener("click", () => review.replaceChildren());
                    confirm.addEventListener("click", async click => {
                        if (!click.isTrusted || !current() || submitting) { return; }
                        const selected = pickers.map(({key, picker}) => ({key, file: picker.files?.item(0)}));
                        if (selected.some(({file}) => !file || file.size === 0 || file.size > 256 * 1024 ||
                            !/^[a-z0-9_][a-z0-9_.-]{0,127}$/i.test(file.name))) {
                            warning.textContent = "请选择符合要求的文件（每份不超过 256 KiB）";
                            return;
                        }
                        submitting = true;
                        confirm.disabled = true;
                        cancel.disabled = true;
                        try {
                            const files: Record<string, {name: string; data: string}> = Object.create(null);
                            for (const {key, file} of selected) {
                                const bytes = new Uint8Array(await file.arrayBuffer());
                                let binary = "";
                                for (let offset = 0; offset < bytes.length; offset += 32768) {
                                    binary += String.fromCharCode(...bytes.subarray(offset, offset + 32768));
                                }
                                files[key] = {name: file.name, data: btoa(binary)};
                            }
                            if (!current()) { return; }
                            const outcome = await binding.session.requestUI("/api/asset/submitExternalFrame", {url, fields, files}) as {text: string};
                            if (current()) {
                                review.textContent = "表单已发送";
                                frame.contentWindow.postMessage({type: "wiki-frame-form-result", text: outcome.text}, "*");
                            }
                        } catch {
                            if (current()) { review.textContent = "表单发送失败"; }
                        } finally {
                            submitting = false;
                        }
                    });
                    review.replaceChildren(details, values, ...pickers.map(item => item.label), warning, confirm, cancel);
                };
                window.addEventListener("message", frameSubmit);
                body.append(review);
            }
            widgetReady = event => {
                if (event.source !== frame.contentWindow || event.origin !== "null" || event.data?.type !== "wiki-widget-ready") { return; }
                window.removeEventListener("message", widgetReady);
                if (current()) {
                    frame.contentWindow.postMessage({type: "wiki-widget-load", html: result.text,
                        scriptBase: result.scriptBase, localForms: kind === "iframe", assets: result.assets, form,
                        previewURL: url}, "*");
                }
            };
            window.addEventListener("message", widgetReady);
            frame.src = "/stage/protyle/js/widget/scoped.html";
            body.append(frame);
            return;
        }
        if (kind === "iframe") {
            inertPreview(body, result.text, result.type === "text/plain");
            return;
        }
        const type = result.type;
        if (!type.startsWith(kind + "/") || typeof result.data !== "string") { throw new Error("invalid_preview"); }
        const data = Uint8Array.from(atob(result.data), c => c.charCodeAt(0));
        previewURL = binding.session.createOwnedURL(new Blob([data], {type}));
        const media = document.createElement(kind === "image" ? "img" : kind);
        media.style.maxWidth = "100%";
        if (media instanceof HTMLMediaElement) { media.controls = true; }
        media.src = previewURL;
        body.append(media);
    } catch {
        if (current()) { body.textContent = window.siyuan.languages.operationFailed; }
    }
}
