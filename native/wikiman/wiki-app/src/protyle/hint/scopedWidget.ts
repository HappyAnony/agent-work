import {Dialog} from "../../dialog";
import {escapeHtml} from "../../util/escape";
import {captureScopedHintRange} from "./scopedRange";
import {commitScopedHint} from "./scopedExternal";
import {widgetPath} from "../../portable/external";

export function selectScopedWidget(protyle: IProtyle, block: HTMLElement, range: Range) {
    const capture = captureScopedHintRange(block, range);
    if (!capture) { return; }
    const binding = protyle.portable;
    const dialog = new Dialog({
        title: escapeHtml(window.siyuan.languages.widget),
        content: '<div class="b3-dialog__content"><input class="b3-text-field fn__block"><div class="b3-list"></div><p role="status"></p></div>',
        width: "min(520px, 85vw)",
    });
    const input = dialog.element.querySelector("input");
    const list = dialog.element.querySelector(".b3-list");
    const status = dialog.element.querySelector("[role=status]");
    input.setAttribute("aria-label", window.siyuan.languages.widget);
    input.focus();
    let generation = 0;
    let selecting = false;
    const current = () => dialog.element.isConnected && binding.session.status === "active";
    async function search() {
        const request = ++generation;
        list.replaceChildren();
        try {
            binding.flushPendingInput();
            await binding.session.flush();
            if (!current() || request !== generation) { return; }
            const widgets = await binding.session.requestUI("/api/widget/list", {k: input.value}) as IBlock[];
            if (!current() || request !== generation) { return; }
            for (const widget of widgets) {
                const src = widgetPath(widget.content);
                if (!src) { continue; }
                const button = document.createElement("button");
                button.type = "button";
                button.className = "b3-list-item";
                button.textContent = widget.name || widget.content;
                button.addEventListener("click", async () => {
                    if (selecting) { return; }
                    selecting = true;
                    try {
                        await binding.session.requestUI("/api/widget/preview", {name: widget.content, validateOnly: true});
                        if (!current()) { return; }
                        const prepared = capture(protyle);
                        if (!prepared) { return; }
                        const parsed = document.createElement("template");
                        parsed.innerHTML = protyle.lute.SpinBlockDOM(`<iframe src="${src}" data-subtype="widget"></iframe>`);
                        const inserted = parsed.content.firstElementChild as HTMLElement;
                        if (inserted?.dataset.type !== "NodeWidget" || !inserted.dataset.nodeId) { return; }
                        dialog.destroy();
                        await commitScopedHint(protyle, block.dataset.nodeId, prepared.clone, inserted);
                    } catch {
                        if (current()) { status.textContent = window.siyuan.languages.operationFailed; }
                    } finally { selecting = false; }
                });
                list.append(button);
            }
            status.textContent = list.childElementCount ? "" : window.siyuan.languages.emptyContent;
        } catch {
            if (current() && request === generation) { status.textContent = window.siyuan.languages.operationFailed; }
        }
    }
    input.addEventListener("input", () => { void search(); });
    dialog.bindInput(input, () => list.querySelector("button")?.click());
    void search();
}
