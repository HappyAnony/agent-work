/// <reference types="./src/types" />
import { Wnd } from "./Wnd";
import { Model } from "./Model";
export declare class Tab {
    parent: Wnd;
    id: string;
    headElement: HTMLElement;
    panelElement: HTMLElement;
    callback: (tab: Tab) => void;
    model: Model;
    title: string;
    icon: string;
    docIcon: string;
    constructor(options: ITab);
    updateTitle(title: string): void;
    addModel(model: Model): void;
    pin(): void;
    setDocIcon(icon: string): void;
    unpin(): void;
    close(): void;
}
