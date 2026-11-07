import {fetchSyncPost} from "../../util/fetch";
import {upDownHint} from "../../util/upDownHint";
import {focusByRange, getSelectionPosition} from "../util/selection";
import {insertHTML} from "../util/insertHTML";
import {getContenteditableElement} from "../wysiwyg/getBlock";
import {setPosition} from "../../util/setPosition";

export function bindScopedTemplates(panel: HTMLElement, protyle: IProtyle, block: HTMLElement, range: Range) {
    const binding = protyle.portable;
    const list = panel.querySelector<HTMLElement>(".b3-list");
    const input = panel.querySelector("input");
    const preview = panel.firstElementChild.lastElementChild as HTMLElement;
    const anchor = range.cloneRange();
    const initial = block.innerHTML;
    let listing = 0, rendering = 0, inserting = false;
    const open = () => panel.isConnected && !panel.classList.contains("fn__none") &&
        binding.session.status === "active" && protyle.wysiwyg.element.contains(block);
    const close = () => {
        panel.classList.add("fn__none");
        if (protyle.wysiwyg.element.contains(anchor.startContainer)) { focusByRange(anchor); }
    };
    preview.setAttribute("inert", "");
    preview.classList.add("b3-typography");
    preview.style.display = "";
    preview.style.width = "min(520px, 50vw)";

    async function render(path: string, previewOnly: boolean) {
        const request = ++rendering;
        if (!previewOnly) {
            binding.flushPendingInput();
            await binding.session.flush();
            if (!open() || request !== rendering) { return; }
        }
        const response = await fetchSyncPost("/api/template/render", {id: binding.document.docId, path, preview: previewOnly});
        if (!open() || request !== rendering || response.code !== 0) { return; }
        const html = previewOnly ? await binding.prepareReadOnlyUIFragment(response.data.content, binding.document) :
            await binding.prepareUIFragment(response.data.content, binding.document);
        if (!open() || request !== rendering) { return; }
        if (previewOnly) {
            preview.innerHTML = html;
        } else if (binding.canRestoreHistory && !protyle.disabled && block.innerHTML === initial &&
            block.contains(anchor.startContainer)) {
            close();
            protyle.toolbar.range = anchor;
            insertHTML(html, protyle, getContenteditableElement(block)?.textContent.trim() === "", true);
        } else {
            binding.reportUnavailableTool();
        }
    }

    function choose(row: HTMLElement, previewOnly = true) {
        if (!row?.dataset.value || inserting) { return; }
        if (!previewOnly) { inserting = true; }
        void render(row.dataset.value, previewOnly).catch(() => binding.reportUnavailableTool())
            .finally(() => { if (!previewOnly) { inserting = false; } });
    }

    async function search() {
        const request = ++listing;
        rendering++;
        preview.replaceChildren();
        list.replaceChildren();
        const response = await fetchSyncPost("/api/search/searchTemplate", {k: input.value});
        if (!open() || request !== listing || response.code !== 0) { return; }
        const items = response.data?.blocks as Array<{path: string}> | undefined;
        for (const item of items ?? []) {
            if (typeof item.path !== "string") { continue; }
            const row = document.createElement("div");
            row.className = "b3-list-item";
            row.dataset.value = item.path;
            const text = document.createElement("span");
            text.className = "b3-list-item__text";
            text.textContent = item.path.split("/").pop();
            row.append(text);
            list.append(row);
        }
        if (list.firstElementChild) {
            list.firstElementChild.classList.add("b3-list-item--focus");
            choose(list.firstElementChild as HTMLElement);
        } else {
            list.textContent = window.siyuan.languages.emptyContent;
        }
    }

    input.addEventListener("input", (event: InputEvent) => { if (!event.isComposing) { void search(); } });
    input.addEventListener("compositionend", () => { void search(); });
    input.addEventListener("keydown", event => {
        event.stopPropagation();
        if (event.isComposing) { return; }
        if (event.key === "Enter") { choose(list.querySelector(".b3-list-item--focus"), false); }
        else if (event.key === "Escape") { close(); }
        else if (event.key === "ArrowUp" || event.key === "ArrowDown") { choose(upDownHint(list, event)); }
        else { return; }
        event.preventDefault();
    });
    list.addEventListener("click", event => choose((event.target as Element).closest(".b3-list-item"), false));
    list.addEventListener("pointerover", event => {
        const row = (event.target as Element).closest<HTMLElement>(".b3-list-item");
        if (!row || row.contains(event.relatedTarget as Node)) { return; }
        list.querySelector(".b3-list-item--focus")?.classList.remove("b3-list-item--focus");
        row.classList.add("b3-list-item--focus");
        choose(row);
    });
    for (const [direction, key] of [["previous", "ArrowUp"], ["next", "ArrowDown"]]) {
        panel.querySelector(`[data-type="${direction}"]`).addEventListener("click", () =>
            input.dispatchEvent(new KeyboardEvent("keydown", {key})));
    }
    const position = getSelectionPosition(block, anchor);
    setPosition(panel, position.left, position.top + 18, 26);
    input.select();
    void search();
}
