import { App } from "../index";
export declare const keymap: {
    element: Element;
    _genItem(keymap: Record<string, Config.IKey>, keys: string): string;
    genHTML(app: App): string;
    _setkeymap(app: App): void;
    search(value: string, keymapString: string): void;
    _toggleSearchItem(editorKeymapElement: HTMLElement, value: string, keymapString: string): void;
    _getTip(element: HTMLElement): string;
    bindEvent(app: App): void;
    _getKeymapString(event: KeyboardEvent): string;
};
