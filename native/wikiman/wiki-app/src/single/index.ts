import {ensureSingleLayoutStyleElement} from "./layoutStyle";
/**
 * Wiki Single Notebook Mode — bundle entry.
 *
 * Loaded inside an iframe served at /stage/build/single/. Performs four-branch
 * boot validation against /api/system/getConf, then mounts a single Protyle
 * editor against the kernel's unique root document. On any failure, renders
 * an inline fallback message in the document body.
 *
 * See docs/feature/siyuan/Engineering/fe/05-boot-entry.md for the spec.
 *
 * MUST NOT import from:
 *   - ../layout/    ../layout/dock/    (no Files/Dock/topBar/Layouts)
 *   - ../menus/     ../dialog/         (except internal Protyle dialogs under ../protyle/dialog/)
 *   - ../window/    ../mobile/
 */

import {Constants} from "../constants";
import {fetchPost} from "../util/fetch";
import {processMessage} from "../util/processMessage";
import {genUUID} from "../util/genID";  // verified: src/util/genID.ts:1 exports genUUID
import {Protyle} from "../protyle";
import {Menu} from "../menus/Menu";
import {addScript, addScriptSync} from "../protyle/util/addScript";
import {getLocalStorage} from "../protyle/util/compatibility";
import {hideElements} from "../protyle/ui/hideElements";
import {initAssets, loadAssets, setInlineStyle} from "../util/assets";
import {renderSnippet} from "../config/util/snippets";
import {correctHotkey} from "../boot/globalEvent/commonHotkey";
import {SingleOutline} from "./SingleOutline";
import {SingleDocHeaderMeta} from "./SingleDocHeaderMeta";

import "../assets/scss/base.scss";  // §EA-M5-8: confirm canonical SCSS entry

// ─── Module-scoped runtime state ──────────────────────────────────────────────

let protyleInstance: Protyle | null = null;
let singleOutline: SingleOutline | null = null;
let singleDocHeaderMeta: SingleDocHeaderMeta | null = null;
let webSocket: WebSocket | null = null;
let bootStartTime = 0;
let bootTimer: number | null = null;

const BOOT_TIMEOUT_MS = 10000;  // §EA-M5-2: 10s timeout for the getConf race
const APP_ID = Constants.WIKI_APPID;
const WS_CLIENT_ID = genUUID();

function readTreeDocTitleRenamedMessage(event: MessageEvent): { title: string } | null {
    const data = event.data as unknown;
    if (!data || typeof data !== "object") {
        return null;
    }

    const payload = data as {type?: unknown; title?: unknown};
    if (payload.type !== "wikiman:tree-doc-title-renamed" || typeof payload.title !== "string") {
        return null;
    }

    const title = payload.title.trim();
    return title ? {title} : null;
}

function bindHostTitleRenameMessages(): void {
    window.addEventListener("message", (event: MessageEvent) => {
        const message = readTreeDocTitleRenamedMessage(event);
        if (!message || !protyleInstance) {
            return;
        }

        const protyle = protyleInstance.protyle;
        if (!protyle.notebookId || !protyle.path || !protyle.block.rootID) {
            return;
        }

        protyle.title?.setTitle(message.title);
        document.title = message.title;
        fetchPost("/api/filetree/renameDoc", {
            notebook: protyle.notebookId,
            path: protyle.path,
            title: message.title,
        });
    });
}

function bindSinglePanelDismiss(protyle: IProtyle): void {
    window.addEventListener("pointerdown", (event: PointerEvent) => {
        const target = event.target as HTMLElement | null;
        if (!target) {
            return;
        }

        if (!target.closest(".b3-menu") && !target.closest('[data-menu="true"]')) {
            window.siyuan.menus.menu.remove();
        }

        if (
            target.closest(".protyle-hint") ||
            target.closest(".protyle-util") ||
            target.closest(".protyle-toolbar")
        ) {
            return;
        }

        hideElements(["hint", "util"], protyle);
    }, true);
}

// ─── Fallback rendering (PRD §6, Architecture §05 §10) ────────────────────────

type FallbackVariant =
    | "kernel-unreachable"
    | "kernel-error"
    | "not-single"
    | "bootstrap-incomplete"
    | "editor-init-failed";

interface FallbackPayload {
    title: string;
    detail: string;
    hint?: string;
    errorCode?: string;
}

const FALLBACKS: Record<FallbackVariant, FallbackPayload> = {
    "kernel-unreachable": {
        title: "Single mode unavailable",
        detail: "Kernel unreachable. The /api/system/getConf call failed or timed out.",
        hint: "Check that the kernel process is running and the iframe URL points at the correct origin/port.",
    },
    "kernel-error": {
        title: "Single mode unavailable",
        detail: "Kernel returned an error response.",
        hint: "Check the kernel log for details.",
    },
    "not-single": {
        title: "Single mode unavailable",
        detail: "Kernel was not started with --single.",
        hint: "Restart the kernel with the --single flag (or set WIKI_SINGLE_NOTEBOOK=1) and reload this iframe.",
    },
    "bootstrap-incomplete": {
        title: "Single mode unavailable",
        detail: "Kernel single-mode bootstrap is incomplete — notebook or root document ID is missing.",
        hint: "Check the kernel log for `event=single_mode_bootstrap_failed` and resolve the underlying error.",
    },
    "editor-init-failed": {
        title: "Single mode unavailable",
        detail: "Editor initialization failed.",
        hint: "Open the browser DevTools console for the underlying error.",
    },
};

// Minimal inline CSS — uses existing siyuan SCSS variables so it matches theme.
const FALLBACK_STYLE = `
    position: fixed;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 24px;
    box-sizing: border-box;
    font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
    color: var(--b3-theme-on-background, #333);
    background: var(--b3-theme-background, #fff);
    z-index: 9999;
`;

function renderFallback(variant: FallbackVariant, errorCode?: string, errorMsg?: string): void {
    const payload = FALLBACKS[variant];
    const container = document.createElement("div");
    container.id = "single-mode-fallback";
    container.setAttribute("style", FALLBACK_STYLE);

    const title = document.createElement("h1");
    title.textContent = payload.title;
    title.style.cssText = "font-size: 20px; margin: 0 0 12px; font-weight: 600;";

    const detail = document.createElement("p");
    detail.textContent = payload.detail + (errorCode ? ` (code=${errorCode})` : "") + (errorMsg ? ` — ${errorMsg}` : "");
    detail.style.cssText = "font-size: 14px; margin: 0 0 8px; max-width: 480px; text-align: center;";

    container.appendChild(title);
    container.appendChild(detail);

    if (payload.hint) {
        const hint = document.createElement("p");
        hint.textContent = payload.hint;
        hint.style.cssText = "font-size: 12px; margin: 0; max-width: 480px; text-align: center; opacity: 0.7;";
        container.appendChild(hint);
    }

    // Clear any partial Protyle DOM that may have rendered before the throw.
    const existingEditor = document.getElementById("editor");
    if (existingEditor) {
        existingEditor.innerHTML = "";
    }
    document.body.appendChild(container);

    console.error(`[single-mode] fallback=${variant}`, {errorCode, errorMsg});
}

// ─── window.siyuan minimal stub (Framework §11.5; src/index.ts:53 pattern) ────

function buildMinimalApp(): /* App-shaped */ any {
    // Cannot import App from "../index" — that file's bottom calls `new App()`
    // and bootstraps the full multi-notebook desktop UI on import. Build a
    // minimal duck-typed object satisfying just the surface Protyle reads.
    // §EA-M5-4 documents the chosen subset.
    return {
        appId: APP_ID,
        plugins: [],  // single mode does not load plugins; Protyle iterates app.plugins.forEach
    };
}

async function loadLanguages(configData: any): Promise<IObject> {
    const lang = configData?.appearance?.lang || "en_US";
    const response = await fetch(`/appearance/langs/${lang}.json?v=${Constants.WIKI_VERSION}`);
    if (!response.ok) {
        throw new Error(`Failed to load language file: ${response.status} ${response.statusText}`);
    }
    return await response.json();
}

async function loadEmojis(): Promise<IEmoji[]> {
    const response = await new Promise<IWebSocketData>((resolve) => {
        fetchPost("/api/system/getEmojiConf", {}, (emojiResponse: IWebSocketData) => {
            resolve(emojiResponse);
        });
    });
    if (response.code !== 0) {
        throw new Error(`Failed to load emoji config: ${response.code} ${response.msg || ""}`.trim());
    }
    return response.data as IEmoji[];
}

async function loadProtyleRuntime(): Promise<void> {
    await addScriptSync(`${Constants.PROTYLE_CDN}/js/lute/lute.min.js?v=${Constants.WIKI_VERSION}`, "protyleLuteScript");
    void addScript(`${Constants.PROTYLE_CDN}/js/protyle-html.js?v=${Constants.WIKI_VERSION}`, "protyleWcHtmlScript");
}

async function loadStorageDefaults(): Promise<void> {
    await new Promise<void>((resolve) => {
        getLocalStorage(() => {
            resolve();
        });
    });
}

function ensureCommonMenuElement(): void {
    if (document.getElementById("commonMenu")) {
        return;
    }
    const menuElement = document.createElement("div");
    menuElement.id = "commonMenu";
    menuElement.className = "b3-menu fn__none";
    menuElement.innerHTML = '<div class="b3-menu__title fn__none"><svg class="b3-menu__icon"><use xlink:href="#iconLeft"></use></svg> <span class="b3-menu__label"></span></div><div class="b3-menu__items"></div>';
    document.body.appendChild(menuElement);
}

function ensureTooltipElement(): void {
    if (document.getElementById("tooltip")) {
        return;
    }
    const tooltipElement = document.createElement("div");
    tooltipElement.id = "tooltip";
    tooltipElement.className = "tooltip fn__none";
    document.body.appendChild(tooltipElement);
}

function ensureMessageElement(): void {
    if (document.getElementById("message")) {
        return;
    }
    const messageElement = document.createElement("div");
    messageElement.id = "message";
    messageElement.className = "b3-snackbars";
    messageElement.innerHTML = '<div class="fn__flex-1"></div>';
    document.body.appendChild(messageElement);
}

function ensurePluginsStyleElement(): void {
    if (document.getElementById("pluginsStyle")) {
        return;
    }
    const pluginsStyleElement = document.createElement("style");
    pluginsStyleElement.id = "pluginsStyle";
    document.head.appendChild(pluginsStyleElement);
}

function ensureSingleHostElements(): void {
    ensureSingleLayoutStyleElement();
    ensurePluginsStyleElement();
    ensureCommonMenuElement();
    ensureTooltipElement();
    ensureMessageElement();
}

async function applySingleAppearance(configData: any): Promise<void> {
    loadAssets(configData.appearance);
    initAssets();
    await setInlineStyle();
    renderSnippet();
}

function initSingleGlobalHotkeys(app: any): void {
    correctHotkey(app);
    window.addEventListener("blur", () => {
        (window as any).siyuan.ctrlIsPressed = false;
        (window as any).siyuan.shiftIsPressed = false;
        (window as any).siyuan.altIsPressed = false;
    });
}

function bootstrapWindowWiki(app: any, configData: any, languages: IObject, emojis: IEmoji[]): void {
    // Mirror the field set initialized in src/index.ts:53-XX so Protyle internals
    // that hit window.siyuan.* don't crash. Everything is empty/default; the
    // ws field is filled in by openWebSocket() below.
    (window as any).siyuan = {
        zIndex: 10,
        transactions: [],
        reqIds: {},
        backStack: [],
        layout: {},
        dialogs: [],
        blockPanels: [],
        closedTabs: [],
        ctrlIsPressed: false,
        altIsPressed: false,
        config: configData,
        storage: {},
        languages,
        emojis,
        user: undefined,
        notebooks: [],
        menus: undefined,
        isPublish: false,
        ws: undefined,  // set by openWebSocket
    };
    ensureSingleHostElements();
    (window as any).siyuan.menus = {
        menu: new Menu(),
    };
}

// ─── Inline WebSocket (Architecture §05 §4.2; URL pattern from Model.ts:39-40) ─

interface MinimalWsHandle {
    ws: WebSocket;
    app: any;
    send: (cmd: string, param: Record<string, unknown>) => void;
}

function openWebSocket(app: any): MinimalWsHandle {
    // URL pattern verbatim from src/layout/Model.ts:39-40 (do NOT import Model).
    // §EA-M5-3 explains the inline choice vs the framework's forbidden-import rule.
    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    const wsUrl = `${scheme}://${window.location.host}/ws?app=${APP_ID}&id=${WS_CLIENT_ID}&type=main`;

    const ws = new WebSocket(wsUrl);
    webSocket = ws;

    ws.onmessage = (event: MessageEvent) => {
        try {
            const data = processMessage(JSON.parse(event.data as string));
            singleOutline?.handleWsMessage(data as IWebSocketData);
            singleDocHeaderMeta?.handleWsMessage(data as IWebSocketData);
            // Phase 1 single mode subscribes to NO cross-document push events.
            // Protyle's own onmessage subscriptions (block transactions, reload-doc)
            // are wired by Protyle internals during construction. M5 stays passive
            // for any other event type — log at DEBUG level only.
            if (data && (data as any).cmd) {
                // console.debug("[single-mode] ws cmd=", (data as any).cmd);
            }
        } catch (e) {
            console.warn("[single-mode] ws onmessage parse failed:", e);
        }
    };

    ws.onclose = (ev: CloseEvent) => {
        if (ev.reason && ev.reason.indexOf("close websocket") < 0) {
            console.warn("[single-mode] WebSocket closed unexpectedly:", ev);
            // No reconnect attempt in Phase 1; Protyle's own retry-on-fetchPost
            // covers data integrity. If kernel comes back up, user can refresh.
        }
    };

    ws.onerror = (err) => {
        console.warn("[single-mode] WebSocket error:", err);
    };

    return {
        ws,
        app,
        send: (cmd: string, param: Record<string, unknown>) => {
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({cmd, param}));
            }
        },
    };
}

// ─── Phase 4: Protyle mount ──────────────────────────────────────────────────

function mountProtyle(app: any, rootDocID: string): boolean {
    const editorEl = document.getElementById("editor");
    if (!editorEl) {
        renderFallback("editor-init-failed", undefined, "Element #editor not found in template.");
        return false;
    }

    try {
        protyleInstance = new Protyle(app, editorEl, {
            // Architecture §05 §4.2 + Framework §5.5 reference these options.
            blockId: rootDocID,
            action: ["cb-get-context"],  // existing wiki-app convention for "open this block with context"
            render: {
                breadcrumb: false,
                background: true,
                title: true,
                // gutter / scroll left as Protyle defaults (per §EA-M5-5)
            },
        } as any);  // cast: IProtyleOptions's exact shape is enforced by Protyle's runtime Options merger; minor type drift is acceptable
        (window as any).siyuan.singleProtyle = protyleInstance.protyle;
        bindSinglePanelDismiss(protyleInstance.protyle);
        singleOutline?.destroy();
        singleDocHeaderMeta?.destroy();
        singleDocHeaderMeta = new SingleDocHeaderMeta({
            blockId: rootDocID,
            protyle: protyleInstance.protyle,
        });
        singleOutline = new SingleOutline({
            blockId: rootDocID,
            protyle: protyleInstance.protyle,
        });
        return true;
    } catch (e) {
        renderFallback("editor-init-failed", undefined, e instanceof Error ? e.message : String(e));
        return false;
    }
}

// ─── Phase 1-5: orchestrator ─────────────────────────────────────────────────

async function bootSingleMode(): Promise<void> {
    bootStartTime = Date.now();
    bindHostTitleRenameMessages();

    // Phase 1: fetchPost with timeout race (Framework §11.13 — fetchPost is callback-based; wrap with setTimeout)
    const conf = await new Promise<IWebSocketData | null>((resolve) => {
        let settled = false;

        bootTimer = window.setTimeout(() => {
            if (settled) return;
            settled = true;
            bootTimer = null;
            resolve(null);  // null signals timeout to caller
        }, BOOT_TIMEOUT_MS);

        fetchPost("/api/system/getConf", {}, (response: IWebSocketData) => {
            if (settled) return;
            settled = true;
            if (bootTimer !== null) {
                clearTimeout(bootTimer);
                bootTimer = null;
            }
            resolve(response);
        });
    });

    // Branch A: timeout / null response
    if (conf == null) {
        renderFallback("kernel-unreachable");
        return;
    }

    // Branch B: non-success code
    if (conf.code !== 0) {
        renderFallback("kernel-error", String(conf.code), conf.msg);
        return;
    }

    // Defensive: missing payload structure (treat as kernel-error rather than not-single
    // so operator sees the real cause — a malformed getConf payload — not a misleading
    // "kernel not in --single" message).
    const system: Config.ISystem | undefined = conf.data?.conf?.system;
    if (!system) {
        renderFallback("kernel-error", undefined, "getConf payload missing data.conf.system");
        return;
    }

    // Branch C: not single mode
    if (system.single !== true) {
        renderFallback("not-single");
        return;
    }

    // Branch D: bootstrap incomplete (M3 set Mode=true but failed before populating IDs)
    if (!system.singleNotebookBoxID || !system.singleNotebookRootDocID) {
        renderFallback("bootstrap-incomplete");
        return;
    }

    const rootDocID: string = system.singleNotebookRootDocID;

    let languages: IObject;
    let emojis: IEmoji[];
    try {
        languages = await loadLanguages(conf.data.conf);
        emojis = await loadEmojis();
        await loadProtyleRuntime();
    } catch (e) {
        renderFallback("editor-init-failed", undefined, e instanceof Error ? e.message : String(e));
        return;
    }

    // Phase 2: window.siyuan stub + minimal app
    const app = buildMinimalApp();
    bootstrapWindowWiki(app, conf.data.conf, languages, emojis);
    try {
        await loadStorageDefaults();
        await applySingleAppearance(conf.data.conf);
    } catch (e) {
        renderFallback("editor-init-failed", undefined, e instanceof Error ? e.message : String(e));
        return;
    }
    initSingleGlobalHotkeys(app);

    // Phase 3: WebSocket
    const wsHandle = openWebSocket(app);
    (window as any).siyuan.ws = wsHandle;

    // Phase 4: Protyle mount
    if (!mountProtyle(app, rootDocID)) {
        return;  // fallback already rendered
    }

    // Phase 5: ready
    console.info(
        `[single-mode] ready boxID=${system.singleNotebookBoxID} rootDocID=${rootDocID} unmounted=${system.unmountedBoxes?.length ?? 0} ignored=${system.ignoredDocs?.length ?? 0} boot=${Date.now() - bootStartTime}ms`,
    );
}

// ─── Module-load: self-invoke ────────────────────────────────────────────────

if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => {
        void bootSingleMode();
    });
} else {
    void bootSingleMode();
}
