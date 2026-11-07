import {setPosition} from "../../../util/setPosition";

export function createScopedCellPanel(cell: HTMLElement, html: string) {
    const panel = document.createElement("div");
    panel.className = "av__panel";
    panel.style.zIndex = String(++window.siyuan.zIndex);
    panel.innerHTML = '<div class="b3-dialog__scrim" data-type="close"></div><div class="b3-menu" style="min-width:200px"></div>';
    const menu = panel.lastElementChild as HTMLElement;
    menu.innerHTML = html;
    document.body.append(panel);
    const position = () => {
        const rect = cell.getBoundingClientRect();
        setPosition(menu,rect.left,rect.bottom,rect.height);
    };
    return {panel,menu,position};
}
