const parentOrigin = new URL(location.href).origin;
async function load(event) {
    if (event.source !== parent || event.origin !== parentOrigin) { return; }
    if (event.data?.type === "wiki-plugin-load" && typeof event.data.code === "string" &&
        event.data.code.length <= 512 * 1024) {
        window.removeEventListener("message", load);
        await loadPlugin(event.data, event.ports[0]);
        return;
    }
    if (event.data?.type !== "wiki-widget-load" ||
        typeof event.data.html !== "string" || event.data.html.length > 512 * 1024) {
        return;
    }
    window.removeEventListener("message", load);
    const source = document.createElement("template");
    source.innerHTML = event.data.html;
    const scripts = Array.from(source.content.querySelectorAll("script"));
    scripts.forEach(script => script.remove());
    const assets = new Map(Array.isArray(event.data.assets) ? event.data.assets.map(item => [item.name, item]) : []);
    source.content.querySelectorAll("link,img,audio,video,source").forEach(node => {
        const asset = assets.get(node.getAttribute("data-ironman-verified"));
        node.removeAttribute("data-ironman-verified");
        node.removeAttribute("srcset");
        node.removeAttribute("poster");
        if (node.localName === "link") {
            if (asset?.type === "text/css") {
                const style = document.createElement("style");
                style.textContent = new TextDecoder().decode(Uint8Array.from(atob(asset.data), char => char.charCodeAt(0)));
                node.replaceWith(style);
            } else { node.remove(); }
        } else {
            const category = node.localName === "source" ? node.parentElement?.localName :
                node.localName === "img" ? "image" : node.localName;
            if (asset?.type.startsWith(category + "/")) {
                node.setAttribute("src", "data:" + asset.type + ";base64," + asset.data);
            } else if (event.data.localForms === true && !node.getAttribute("src")?.startsWith("data:")) {
                node.removeAttribute("src");
            }
        }
    });
    source.content.querySelectorAll("iframe,object,embed,meta,base").forEach(node => node.remove());
    const formPolicy = event.data.form;
    if (formPolicy?.action) {
        window.addEventListener("message", result => {
            if (result.source !== parent || result.origin !== parentOrigin ||
                result.data?.type !== "wiki-frame-form-result" || typeof result.data.text !== "string") { return; }
            const status = document.createElement("p");
            status.setAttribute("role", "status");
            status.textContent = result.data.text;
            document.body.append(status);
        });
    }
    source.content.querySelectorAll("form").forEach(form => {
        if (event.data.localForms !== true) { form.remove(); return; }
        let permittedAction = false;
        if (formPolicy?.action && form.getAttribute("method")?.toLowerCase() === "post") {
            try {
                permittedAction = new URL(form.getAttribute("action") || "", event.data.previewURL).href === formPolicy.action &&
                    Object.keys(formPolicy.files || {}).every(name => form.elements.namedItem(name)?.type === "file");
            } catch { /* Invalid actions remain local. */ }
        }
        ["action", "target", "method", "enctype"].forEach(name => form.removeAttribute(name));
        form.querySelectorAll("input[type=file]").forEach(input => input.remove());
        form.querySelectorAll("[formaction],[formtarget],[formmethod],[formenctype]").forEach(node => {
            ["formaction", "formtarget", "formmethod", "formenctype"].forEach(name => node.removeAttribute(name));
        });
        const submit = (submitter, trusted) => {
            if (!form.reportValidity()) { return; }
            form.dispatchEvent(new SubmitEvent("submit", {bubbles: true, cancelable: true, submitter}));
            if (!trusted || !permittedAction) { return; }
            const values = new FormData(form);
            const fields = Object.create(null);
            for (const name of formPolicy.fields || []) {
                const control = form.elements.namedItem(name);
                const items = values.getAll(name);
                if (!control || !(control instanceof HTMLElement) || ["file", "password"].includes(control.type) ||
                    control.type === "hidden" && !formPolicy.hiddenFields?.includes(name) ||
                    items.length !== 1 || typeof items[0] !== "string" || items[0].length > 2048) { return; }
                fields[name] = items[0];
            }
            parent.postMessage({type: "wiki-frame-form", fields}, parentOrigin);
        };
        form.addEventListener("submit", event => event.preventDefault(), true);
        form.addEventListener("click", event => {
            const button = event.target.closest("button,input");
            if (!button || button.form !== form || button.type !== "submit") { return; }
            event.preventDefault();
            submit(button, event.isTrusted);
        });
        form.addEventListener("keydown", event => {
            if (event.key !== "Enter" || event.isComposing || !(event.target instanceof HTMLInputElement) ||
                !["text", "search", "email", "url", "tel", "number"].includes(event.target.type)) { return; }
            event.preventDefault();
            submit(null, event.isTrusted);
        });
    });
    source.content.querySelectorAll("a").forEach(node => node.removeAttribute("href"));
    document.body.replaceChildren(source.content);
    const base = typeof event.data.scriptBase === "string" &&
        /^\/stage\/protyle\/js\/widget\/resources\/[a-f0-9]{48}\/$/.test(event.data.scriptBase)
        ? new URL(event.data.scriptBase, parentOrigin) : null;
    for (const script of scripts) {
        try {
            const classic = ["", "text/javascript", "application/javascript"].includes(script.type);
            if (script.hasAttribute("src")) {
                if (!base || !classic && script.type !== "module") { continue; }
                const url = new URL(script.getAttribute("src"), base);
                if (!url.href.startsWith(base.href) || url.search || url.hash) { continue; }
                await new Promise((resolve, reject) => {
                    const trusted = document.createElement("script");
                    if (!classic) { trusted.type = "module"; }
                    trusted.crossOrigin = "anonymous";
                    trusted.src = url.href;
                    trusted.onload = resolve;
                    trusted.onerror = reject;
                    document.head.append(trusted);
                });
            } else if (classic) {
                (0, eval)(script.textContent);
            }
        } catch {
            const warning = document.createElement("p");
            warning.textContent = "Widget script could not start.";
            document.body.append(warning);
        }
    }
}
async function loadPlugin({name, code, storage, protyle}, storagePort) {
    const hostActions = [];
    let notification = "";
    let panel;
    let panelChanged = false;
    const tabTypes = new Map();
    function closePanel(notify = true) {
        if (!panel) { return; }
        const previous = panel;
        panel = undefined;
        previous.element.remove();
        try { previous.destroy?.(); } catch { /* Panel has already been closed. */ }
        if (notify) { parent.postMessage({type: "wiki-plugin-panel-close"}, parentOrigin); }
    }
    function showPanel(title, destroy) {
        closePanel(false);
        const element = document.createElement("section");
        const header = document.createElement("h2");
        header.textContent = String(title).slice(0, 96);
        const close = document.createElement("button");
        close.textContent = "Close plugin panel";
        close.addEventListener("click", () => closePanel());
        const content = document.createElement("div");
        element.append(header, close, content);
        document.body.replaceChildren(element);
        panel = {element, title: header.textContent, destroy};
        panelChanged = true;
        return content;
    }
    const Setting = class {
        constructor(options = {}) {
            this.options = options;
            this.items = [];
        }
        addItem(item) {
            if (item && this.items.length < 32) { this.items.push(item); }
        }
        open(title) {
            const element = showPanel(title || name, this.options.destroyCallback);
            for (const item of this.items) {
                const row = document.createElement("label");
                row.style.display = "block";
                row.textContent = String(item.title ?? "").slice(0, 256);
                if (item.description) {
                    const description = document.createElement("p");
                    description.textContent = String(item.description).slice(0, 512);
                    row.append(description);
                }
                const action = item.actionElement || item.createActionElement?.();
                if (action instanceof HTMLElement) { row.append(action); }
                element.append(row);
            }
            if (typeof this.options.confirmCallback === "function") {
                const save = document.createElement("button");
                save.textContent = "Save plugin settings";
                save.addEventListener("click", async () => {
                    save.disabled = true;
                    try {
                        await this.options.confirmCallback();
                        closePanel();
                    } catch {
                        save.disabled = false;
                    }
                });
                element.append(save);
            }
            this.dialog = {element, destroy: () => closePanel()};
        }
    };
    function openCustomPanel(options, title, data, type) {
        let model;
        const element = showPanel(title, () => {
            options.beforeDestroy?.call(model);
            options.destroy?.call(model);
        });
        model = {element, data, type};
        panel.model = model;
        options.init.call(model);
        return model;
    }
    const storageRequests = new Map();
    let nextStorageRequest = 0;
    if (storagePort) { storagePort.onmessage = event => {
        const request = storageRequests.get(event.data.request);
        if (!request) { return; }
        storageRequests.delete(event.data.request);
        clearTimeout(request.timer);
        if (event.data.ok === true) { request.resolve(event.data.result); }
        else { request.reject(new Error("Plugin storage unavailable")); }
    }; }
    function requestStorage(action, file, value) {
        if (storage !== true || !storagePort || typeof file !== "string" || !/^[a-z0-9_][a-z0-9_.-]{0,127}$/i.test(file) ||
            storageRequests.size >= 8 || action === "save" && (typeof value !== "string" || value.length > 65536)) {
            return Promise.reject(new Error("Plugin storage unavailable"));
        }
        const request = String(++nextStorageRequest);
        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => {
                storageRequests.delete(request);
                reject(new Error("Plugin storage timed out"));
            }, 10000);
            storageRequests.set(request, {timer, resolve, reject});
            storagePort.postMessage({request, action, file, value});
        });
    }
    const Plugin = class {
        constructor(options) {
            this.name = options.name;
            this.displayName = options.displayName;
            this.i18n = {};
            this.protyleSlash = [];
            this.data = Object.create(null);
            const target = new EventTarget();
            this.eventBus = Object.freeze({
                on: (type, listener) => target.addEventListener(type, listener),
                once: (type, listener) => target.addEventListener(type, listener, {once: true}),
                off: (type, listener) => target.removeEventListener(type, listener),
                emit: (type, detail) => target.dispatchEvent(new CustomEvent(type, {detail, cancelable: true})),
            });
        }
        async loadData(file) {
            const value = await requestStorage("load", file);
            this.data[file] = value;
            return value;
        }
        async saveData(file, value) {
            const serialized = typeof value === "object" ? JSON.stringify(value) : String(value);
            const result = await requestStorage("save", file, serialized);
            this.data[file] = value;
            return result;
        }
        async removeData(file) {
            const result = await requestStorage("remove", file);
            delete this.data[file];
            return result;
        }
        addCommand(command) {
            if (hostActions.length >= 12 || typeof command?.callback !== "function" && typeof command?.editorCallback !== "function" &&
                typeof command?.globalCallback !== "function") { return; }
            const key = typeof command.langKey === "string" ? command.langKey.slice(0, 96) : "";
            const label = key ? this.i18n[key] || key : name;
            const callback = typeof command.editorCallback === "function" ? () => command.editorCallback(protyle) : () => command.callback?.();
            hostActions.push({key, label: String(label).slice(0, 96), placement: "command", callback, globalCallback: command.globalCallback});
        }
        addTopBar(options) {
            const button = document.createElement("button");
            if (hostActions.length >= 12 || typeof options?.title !== "string" || typeof options.callback !== "function") {
                return button;
            }
            button.textContent = options.title.slice(0, 96);
            let result;
            button.addEventListener("click", event => {
                try { result = options.callback(event); } catch (error) { result = Promise.reject(error); }
            });
            hostActions.push({label: button.textContent, placement: "toolbar", callback: () => {
                result = undefined;
                button.click();
                return result;
            }});
            return button;
        }
        openSetting() {
            this.setting?.open(this.displayName || this.name);
        }
        addDock(options) {
            if (hostActions.length >= 12 || typeof options?.init !== "function" || typeof options.config?.title !== "string") { return; }
            const label = options.config.title.slice(0, 96);
            hostActions.push({label, placement: "dock", position: options.config.position,
                callback: () => openCustomPanel(options, label, options.data, options.type)});
            return {config: options.config};
        }
        addStatusBar(options) {
            const element = options?.element;
            if (!(element instanceof HTMLElement) || hostActions.length >= 12) { return element; }
            hostActions.push({label: (element.getAttribute("aria-label") || element.textContent || name).slice(0, 96),
                placement: "status", callback: () => element.click()});
            return element;
        }
        addTab(options) {
            if (tabTypes.size >= 12 || typeof options?.init !== "function" || typeof options.type !== "string") { return; }
            const id = this.name + options.type;
            tabTypes.set(id, options);
            return ({data}) => openCustomPanel(options, options.type, data, options.type);
        }
        getOpenedTab() {
            const tabs = Object.create(null);
            for (const [, options] of tabTypes) {
                tabs[options.type] = panel?.model?.type === options.type ? [panel.model] : [];
            }
            return tabs;
        }
    };
    const require = name => {
        if (name === "siyuan") {
            return Object.freeze({Plugin, Setting, openTab: async ({custom} = {}) => {
                const options = tabTypes.get(custom?.id);
                if (options) { return openCustomPanel(options, custom.title, custom.data, options.type); }
            }, showMessage: text => {
                if (typeof text === "string") { notification = text.slice(0, 512); }
            }});
        }
        throw new Error("Unsupported plugin module");
    };
    const module = {exports: {}};
    const commands = [];
    try {
        new Function("require", "module", "exports", code)(require, module, module.exports);
        const Type = module.exports.default || module.exports;
        if (typeof Type !== "function" || !(Type.prototype instanceof Plugin)) { throw new Error("Invalid plugin"); }
        const plugin = new Type({name, displayName: name, i18n: {}});
        await plugin.onload?.();
        await plugin.onLayoutReady?.();
        plugin.eventBus.emit("loaded-protyle-static", {protyle});
        window.addEventListener("message", event => {
            if (event.source !== parent || event.origin !== parentOrigin || event.data?.type !== "wiki-plugin-event" ||
                event.data.name !== "click-editorcontent") { return; }
            plugin.eventBus.emit(event.data.name, event.data.detail);
        });
        commands.push(...hostActions);
        for (const item of plugin.protyleSlash.slice(0, 24 - hostActions.length)) {
            if (!item || typeof item.callback !== "function" || !Array.isArray(item.filter)) { continue; }
            const terms = item.filter.filter(term => typeof term === "string").slice(0, 12).map(term => term.slice(0, 64));
            const label = document.createElement("template");
            label.innerHTML = typeof item.html === "string" ? item.html.slice(0, 4096) : "";
            commands.push({label: (label.content.textContent.trim() || terms[0] || name).slice(0, 96), terms, placement: "slash", callback: item.callback});
        }
    } catch { /* unsupported plugin does not publish commands */ }
    window.addEventListener("message", async event => {
        if (event.source !== parent || event.origin !== parentOrigin || event.data?.type !== "wiki-plugin-run") { return; }
        const {index, request} = event.data;
        if (!Number.isInteger(index) || !commands[index] || typeof request !== "string") { return; }
        let text = "";
        notification = "";
        panelChanged = false;
        try {
            const insert = html => {
                if (typeof html !== "string" || html.length > 16384) { throw new Error("Invalid plugin insertion"); }
                const template = document.createElement("template");
                template.innerHTML = html;
                template.content.querySelectorAll("script,style,template").forEach(node => node.remove());
                text += template.content.textContent;
                if (text.length > 4096) { throw new Error("Plugin insertion too large"); }
            };
            if (event.data.system === true) {
                await commands[index].globalCallback?.();
            } else {
                await commands[index].callback(Object.freeze({insert}), document.createElement("div"));
            }
        } catch { text = ""; notification = ""; }
        parent.postMessage({type: "wiki-plugin-result", request, text, notification,
            panel: panelChanged && panel ? panel.title : undefined}, parentOrigin);
    });
    parent.postMessage({type: "wiki-plugin-commands", commands: commands.map(({key, label, terms = [], placement, position, globalCallback}) =>
        ({key, label, terms, placement, position, global: typeof globalCallback === "function"}))}, parentOrigin);
}
window.addEventListener("message", load);
parent.postMessage({type:"wiki-widget-ready"}, parentOrigin);
