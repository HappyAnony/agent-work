/// <reference types="./src/types" />
export declare class Menu {
    private menu;
    isOpen: boolean;
    element: HTMLElement;
    constructor(id?: string, closeCB?: () => void);
    showSubMenu(subMenuElement: HTMLElement): void;
    addItem(option: IMenu): HTMLElement;
    addSeparator(options?: number | {
        index?: number;
        id?: string;
        ignore?: boolean;
    }, ignoreParam?: boolean): HTMLElement;
    open(options: IPosition): void;
    fullscreen(position?: "bottom" | "all"): void;
    close(): void;
}
