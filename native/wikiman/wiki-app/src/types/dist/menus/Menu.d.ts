/// <reference types="./src/types" />
export declare class Menu {
    element: HTMLElement;
    data: any;
    removeCB: () => void;
    private wheelEvent;
    constructor();
    showSubMenu(subMenuElement: HTMLElement): void;
    private preventDefault;
    addItem(option: IMenu): HTMLElement;
    removeScrollEvent(): void;
    remove(isKeyEvent?: boolean): void;
    append(element?: HTMLElement, index?: number): void;
    popup(options: IPosition): void;
    fullscreen(position?: "bottom" | "all"): void;
}
export declare class MenuItem {
    element: HTMLElement;
    constructor(options: IMenu);
}
export declare const bindMenuKeydown: (event: KeyboardEvent) => boolean;
export declare class subMenu {
    menus: IMenu[];
    constructor();
    addSeparator(index?: number, id?: string): void;
    addItem(menu: IMenu): void;
}
