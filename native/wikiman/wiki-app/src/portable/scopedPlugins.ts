import type {ProtyleBinding} from "./protyle";
import {escapeHtml} from "../util/escape";
import {captureScopedHintRange} from "../protyle/hint/scopedRange";
import {commitScopedHint} from "../protyle/hint/scopedExternal";
import {matchHotKey} from "../protyle/util/hotKey";
import type {PluginHostAction} from "./bridge";

type Command = {id: string; label: string; terms: string[]; hotkey?: string; run: () => Promise<string>};
const commands = new WeakMap<ProtyleBinding, Command[]>();

export function scopedPluginHints(binding: ProtyleBinding, key: string): IHintData[] {
    const needle = key.toLowerCase();
    return (commands.get(binding) || []).filter(command => [command.label, ...command.terms].some(
        term => term.toLowerCase().includes(needle))).map(command => ({
        id: command.id, value: command.id,
        html: `<div class="b3-list-item__text">${escapeHtml(command.label)}</div>`,
    }));
}

export function isScopedPluginHint(binding: ProtyleBinding, id: string) {
    return commands.get(binding)?.some(command => command.id === id) === true;
}

export async function mountScopedPlugins(binding: ProtyleBinding, owner: HTMLElement, toolbar: HTMLElement, content: HTMLElement,
                                        layout?: {workspace: HTMLElement; pane: HTMLElement}, hostLayout = false) {
    if (binding.session.view.mode !== "wikiDesktop") { return; }
    const list: Command[] = [];
    const system = new Map<string, () => void>();
    const hostActions = new Map<string, () => void>();
    const presentation: PluginHostAction[] = [];
    binding.runPluginHostAction = id => {
        if (binding.session.status === "active") { hostActions.get(id)?.(); }
    };
    binding.runPluginSystemShortcut = chord => {
        if (binding.session.status === "active") { system.get(chord)?.(); }
    };
    commands.set(binding, list);
    const cleanup: (() => void)[] = [];
    let disposed = false;
    const keydown = (event: KeyboardEvent) => {
        if (event.defaultPrevented || event.isComposing || event.keyCode === 229 || event.repeat ||
            binding.session.status !== "active" || !(event.target instanceof Element) ||
            event.target.closest("input,textarea,select,button,.protyle-wysiwyg__embed")) { return; }
        const command = list.find(item => item.hotkey && matchHotKey(item.hotkey, event));
        if (!command) { return; }
        event.preventDefault();
        event.stopPropagation();
        void command.run();
    };
    content.addEventListener("keydown", keydown);
    binding.onDetach(() => {
        disposed = true;
        content.removeEventListener("keydown", keydown);
        commands.delete(binding);
        cleanup.forEach(close => close());
    });
    const plugins = await binding.session.requestUI("/api/plugin/loadSandboxed", {}) as {
        name: string; code: string; storage: boolean; hotkeys?: Record<string, string>; systemHotkeys?: Record<string, string>;
    }[];
    if (disposed || !owner.isConnected || !Array.isArray(plugins)) { return; }
    const regions = new Map<string, HTMLElement>();
    const region = (position: string) => {
        const key = position.startsWith("Right") ? "right" : position.startsWith("Bottom") ? "bottom" :
            position === "status" ? "status" : "left";
        if (!layout) { return toolbar; }
        if (!regions.has(key)) {
            const rail = document.createElement("div");
            rail.setAttribute("role", "toolbar");
            rail.setAttribute("aria-label", "Wiki plugin " + key);
            if (key === "left" || key === "right") {
                rail.className = "dock dock--vertical";
                if (key === "right") { layout.workspace.append(rail); } else { layout.workspace.prepend(rail); }
            } else { layout.pane.append(rail); }
            regions.set(key, rail);
            cleanup.push(() => rail.remove());
        }
        return regions.get(key);
    };
    for (const [pluginIndex, plugin] of plugins.slice(0, 8).entries()) {
        const controls = document.createElement("span");
        const status = document.createElement("span");
        status.setAttribute("role", "status");
        const frame = document.createElement("iframe");
        frame.hidden = true;
        const inlineFrameStyle = "width:100%;height:16rem;border:0";
        frame.style.cssText = inlineFrameStyle;
        frame.sandbox.add("allow-scripts");
        frame.referrerPolicy = "no-referrer";
        frame.src = "/stage/protyle/js/widget/scoped.html";
        let started = false;
        let registered = false;
        const storageRequests = new Set<string>();
        const storageChannel = plugin.storage ? new MessageChannel() : undefined;
        const protyle = {block: {rootID: binding.document.docId}, notebookId: binding.document.boxId};
        const click = (event: MouseEvent) => {
            if (!registered || disposed || binding.session.status !== "active" || !(event.target instanceof Element)) { return; }
            const block = event.target.closest<HTMLElement>("[data-node-id]");
            const blockId = block?.dataset.nodeId;
            if (!block || !/^\d{14}-[a-z0-9]{7}$/.test(blockId ?? "") || !content.contains(block) ||
                block.closest(".protyle-wysiwyg__embed")) { return; }
            frame.contentWindow.postMessage({type: "wiki-plugin-event", name: "click-editorcontent", detail: {protyle, blockId}}, "*");
        };
        async function storage(message: {request: string; action: string; file: string; value?: string}) {
            if (!plugin.storage || typeof message.request !== "string" || message.request.length > 64 ||
                storageRequests.has(message.request) || storageRequests.size >= 8 || !/^[a-z0-9_][a-z0-9_.-]{0,127}$/i.test(message.file) ||
                !["load", "save", "remove"].includes(message.action) ||
                (message.action === "save" ? typeof message.value !== "string" || message.value.length > 65536 : message.value !== undefined)) { return; }
            const {request, action, file, value} = message;
            storageRequests.add(request);
            let result: unknown;
            let ok = false;
            try {
                result = await binding.session.pluginStorage({plugin: plugin.name, action, file, value});
                ok = true;
            } catch { /* Reject in the sandbox without disclosing host errors. */ }
            storageRequests.delete(request);
            if (!disposed && binding.session.status === "active") {
                storageChannel.port1.postMessage({request, ok, result});
            }
        }
        if (storageChannel) {
            storageChannel.port1.onmessage = event => { if (!disposed && event.data) { void storage(event.data); } };
        }
        let pending: {request: string; complete: (text: string) => void; timer: number; position?: string};
        const complete = (text: string) => {
            const previous = pending;
            pending = undefined;
            window.clearTimeout(previous.timer);
            previous.complete(text);
        };
        const run = (index: number, system = false, position?: string) => new Promise<string>(resolve => {
            if (pending || disposed || binding.session.status !== "active") { resolve(""); return; }
            const request = crypto.randomUUID();
            pending = {request, complete: resolve, timer: window.setTimeout(() => complete(""), 5000), position};
            frame.contentWindow.postMessage({type: "wiki-plugin-run", index, request, system}, "*");
        });
        const receive = (event: MessageEvent) => {
            if (event.source !== frame.contentWindow || event.origin !== "null" || disposed) { return; }
            if (event.data?.type === "wiki-widget-ready" && !started) {
                started = true;
                frame.contentWindow.postMessage({type: "wiki-plugin-load", ...plugin, protyle}, "*",
                    storageChannel ? [storageChannel.port2] : []);
            } else if (event.data?.type === "wiki-plugin-commands" && started && !registered && Array.isArray(event.data.commands)) {
                registered = true;
                event.data.commands.slice(0, 24).forEach((command, index) => {
                    if (typeof command?.label !== "string" || !Array.isArray(command.terms)) { return; }
                    if (["toolbar", "dock", "status"].includes(command.placement)) {
                        const button = document.createElement("button");
                        button.type = "button";
                        button.textContent = command.label.slice(0, 96);
                        const position = command.placement === "dock" &&
                            /^(LeftTop|LeftBottom|RightTop|RightBottom|BottomLeft|BottomRight)$/.test(command.position) ?
                            command.position : undefined;
                        const invoke = () => { void run(index, false, position); };
                        if (hostLayout && (position || command.placement === "status") && button.textContent) {
                            const id = `p${pluginIndex}-${index}`;
                            const region = position ? position.startsWith("Right") ? "right" : position.startsWith("Bottom") ? "bottom" : "left" : "status";
                            hostActions.set(id, invoke);
                            presentation.push({id, label: button.textContent, region});
                            return;
                        }
                        button.addEventListener("click", invoke);
                        const destination = position ? region(position) : command.placement === "status" ? region("status") : controls;
                        destination.append(button);
                        cleanup.push(() => button.remove());
                        return;
                    }
                    if (command.placement !== "slash" && command.placement !== "command") { return; }
                    if (command.global && command.placement === "command" && typeof command.key === "string" &&
                        Object.hasOwn(plugin.systemHotkeys || {}, command.key)) {
                        const chord = plugin.systemHotkeys[command.key];
                        system.set(chord, () => { void run(index, true); });
                    }
                    list.push({id: `scoped-plugin-${pluginIndex}-${index}`, label: command.label.slice(0, 96),
                        hotkey: command.placement === "command" && typeof command.key === "string" &&
                            Object.hasOwn(plugin.hotkeys || {}, command.key) ? plugin.hotkeys[command.key] : undefined,
                        terms: command.terms.filter(term => typeof term === "string").slice(0, 12), run: () => run(index)});
                });
                binding.pluginSystemShortcuts = Array.from(system.keys());
                binding.reportPluginSystemShortcuts?.(binding.pluginSystemShortcuts);
                if (hostLayout) {
                    binding.pluginHostActions = [...presentation];
                    binding.reportPluginHostActions?.(binding.pluginHostActions);
                }
            } else if (event.data?.type === "wiki-plugin-result" && pending && pending.request === event.data.request) {
                const text = event.data.text;
                const notification = event.data.notification;
                status.textContent = typeof notification === "string" ? notification.slice(0, 512) : "";
                if (typeof event.data.panel === "string") {
                    frame.title = plugin.name + ": " + event.data.panel.slice(0, 96);
                    frame.style.cssText = inlineFrameStyle;
                    const position = pending.position;
                    if (position) {
                        frame.style.cssText += ";position:fixed;z-index:20;background:var(--b3-theme-background);border:1px solid var(--b3-border-color);";
                        if (position.startsWith("Bottom")) {
                            frame.style.cssText += "bottom:2rem;width:45%;";
                            frame.style[position === "BottomLeft" ? "left" : "right"] = "1rem";
                        } else {
                            frame.style.cssText += "width:min(320px, 45%);";
                            frame.style[position.startsWith("Left") ? "left" : "right"] = "2rem";
                            frame.style[position.endsWith("Top") ? "top" : "bottom"] = "2rem";
                        }
                    }
                    frame.hidden = false;
                }
                complete(typeof text === "string" && text.length <= 4096 ? text : "");
            } else if (event.data?.type === "wiki-plugin-panel-close" && registered) {
                frame.hidden = true;
            }
        };
        window.addEventListener("message", receive);
        content.addEventListener("click", click);
        cleanup.push(() => {
            window.removeEventListener("message", receive);
            content.removeEventListener("click", click);
            storageChannel?.port1.close();
            if (pending) { complete(""); }
            frame.remove();
            controls.remove();
        });
        controls.append(status);
        toolbar.append(controls);
        owner.append(frame);
    }
}

export async function applyScopedPluginHint(protyle: IProtyle, id: string, block: HTMLElement, range: Range) {
    const binding = protyle.portable;
    const command = commands.get(binding)?.find(item => item.id === id);
    const capture = captureScopedHintRange(block, range);
    if (!command || !capture) { return; }
    const text = await command.run();
    const result = capture(protyle);
    if (!text || !result || binding.session.status !== "active") { return; }
    result.selection.insertNode(document.createTextNode(text));
    await commitScopedHint(protyle, block.dataset.nodeId, result.clone);
}
