import type {App} from "../index";
import {Constants} from "../constants";
import {Dialog} from "../dialog";
import {focusByRange} from "../protyle/util/selection";
import {appearance} from "./appearance";
import {keymap} from "./keymap";

export function openPortableSettings(app: App) {
    const existing = window.siyuan.dialogs.find(item => item.element.getAttribute("data-key") === Constants.DIALOG_SETTING);
    if (existing) {
        existing.destroy();
        return;
    }
    const languages = window.siyuan.languages;
    const selection = getSelection();
    const range = selection?.rangeCount ? selection.getRangeAt(0).cloneRange() : undefined;
    const dialog = new Dialog({
        content: `<div class="fn__flex-1 fn__flex config__panel" style="overflow:hidden;position:relative">
  <ul class="b3-tab-bar b3-list b3-list--background">
    <div class="config__tab-title resize__move">
      <svg class="b3-list-item__graphic"><use xlink:href="#iconSettings"></use></svg>
      <span class="b3-list-item__text">${languages.config}</span>
    </div>
    <div class="config__tab-hr"></div>
    <li data-name="appearance" class="b3-list-item b3-list-item--focus"><svg class="b3-list-item__graphic"><use xlink:href="#iconTheme"></use></svg><span class="b3-list-item__text">${languages.appearance}</span></li>
    <li data-name="keymap" class="b3-list-item"><svg class="b3-list-item__graphic"><use xlink:href="#iconKeymap"></use></svg><span class="b3-list-item__text">${languages.keymap}</span></li>
  </ul>
  <div class="config__tab-wrap">
    <div class="fn__hr--b resize__move"></div>
    <div class="config__tab-container" data-name="appearance"></div>
    <div class="config__tab-container fn__none" data-name="keymap" style="overflow:scroll"></div>
    <div class="fn__hr--b"></div>
  </div>
</div>`,
        width: "90vw",
        height: "90vh",
        destroyCallback() {
            if (range?.startContainer.isConnected) { focusByRange(range); }
        },
    });
    dialog.element.setAttribute("data-key", Constants.DIALOG_SETTING);
    dialog.element.querySelector<HTMLElement>(".b3-dialog__container").style.maxWidth = "1280px";
    appearance.element = dialog.element.querySelector('.config__tab-container[data-name="appearance"]');
    appearance.element.innerHTML = appearance.genHTML();
    appearance.bindEvent();
    keymap.element = dialog.element.querySelector('.config__tab-container[data-name="keymap"]');
    for (const tab of dialog.element.querySelectorAll<HTMLElement>(".b3-tab-bar li[data-name]")) {
        tab.addEventListener("click", () => {
            const showAppearance = tab.dataset.name === "appearance";
            dialog.element.querySelector(".b3-tab-bar .b3-list-item--focus").classList.remove("b3-list-item--focus");
            tab.classList.add("b3-list-item--focus");
            appearance.element.classList.toggle("fn__none", !showAppearance);
            keymap.element.classList.toggle("fn__none", showAppearance);
            if (!showAppearance && !keymap.element.hasChildNodes()) {
                keymap.element.innerHTML = keymap.genHTML(app);
                keymap.bindEvent(app);
            }
        });
    }
}
