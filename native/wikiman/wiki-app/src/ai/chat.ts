import {Dialog} from "../dialog";
import {isMobile} from "../util/functions";
import {fetchPost, fetchSyncPost} from "../util/fetch";
import {fillContent} from "./actions";

export const AIChat = (protyle: IProtyle, element: Element) => {
    const dialog = new Dialog({
        title: "✨ " + window.siyuan.languages.aiWriting,
        content: `<div class="b3-dialog__content"><textarea class="b3-text-field fn__block"></textarea></div>
<div class="b3-dialog__action">
    <button class="b3-button b3-button--cancel">${window.siyuan.languages.cancel}</button><div class="fn__space"></div>
    <button class="b3-button b3-button--text">${window.siyuan.languages.confirm}</button>
</div>`,
        width: isMobile() ? "92vw" : "520px",
    });
    const inputElement = dialog.element.querySelector("textarea");
    const btnsElement = dialog.element.querySelectorAll(".b3-button");
    dialog.bindInput(inputElement, () => {
        (btnsElement[1] as HTMLButtonElement).click();
    });
    inputElement.focus();
    btnsElement[0].addEventListener("click", () => {
        dialog.destroy();
    });
    const binding = protyle.portable;
    if (binding) {
        const confirm = btnsElement[1] as HTMLButtonElement;
        const original = element.innerHTML;
        const live = () => dialog.element.classList.contains("b3-dialog--open") &&
            binding.session.status === "active" && protyle.wysiwyg.element.contains(element);
        const result = document.createElement("p");
        result.setAttribute("role", "status");
        inputElement.after(result);
        confirm.addEventListener("click", async () => {
            if (!live() || !binding.aiWritingEnabled || !inputElement.value.trim() || confirm.disabled) { return; }
            confirm.disabled = true;
            const prompt = inputElement.value;
            try {
                await binding.session.flush();
                if (!live()) { return; }
                const response = await fetchSyncPost("/api/ai/chatGPT", {msg: prompt});
                if (!live()) { return; }
                if (response.code !== 0 || typeof response.data !== "string") {
                    result.textContent = window.siyuan.languages.operationFailed;
                    return;
                }
                if (prompt.trim() === "Clear context") { dialog.destroy(); return; }
                const markdown = [prompt, response.data].filter(Boolean).join("\n\n");
                const content = await binding.prepareUIFragment(protyle.lute.SpinBlockDOM(markdown), binding.document);
                if (!live() || protyle.disabled || element.innerHTML !== original) { return; }
                dialog.destroy();
                fillContent(protyle, content, [element], true);
            } catch {
                if (live()) { result.textContent = window.siyuan.languages.operationFailed; }
            } finally { confirm.disabled = false; }
        });
        return;
    }
    btnsElement[1].addEventListener("click", () => {
        let inputValue = inputElement.value;
        fetchPost("/api/ai/chatGPT", {
            msg: inputValue,
        }, (response) => {
            dialog.destroy();
            let respContent = "";
            if (response.data && "" !== response.data) {
                respContent = "\n\n" + response.data;
            }
            if (inputValue === "Clear context") {
                inputValue = "";
            }
            fillContent(protyle, `${inputValue}${respContent}`, [element]);
        });
    });
};
