import { Layout } from "./index";
import { Tab } from "./Tab";
import { App } from "../index";
export declare class Wnd {
    #private;
    private app;
    id: string;
    parent?: Layout;
    element: HTMLElement;
    headersElement: HTMLElement;
    children: Tab[];
    resize?: Config.TUILayoutDirection;
    constructor(app: App, resize?: Config.TUILayoutDirection, parentType?: Config.TUILayoutType);
    private isPointWithinLines;
    private updateDragElement;
    showHeading(): void;
    switchTab(target: HTMLElement, pushBack?: boolean, update?: boolean, resize?: boolean, isSaveLayout?: boolean): void;
    addTab(tab: Tab, keepCursor?: boolean, isSaveLayout?: boolean, activeTime?: string): void;
    private renderTabList;
    private removeOverCounter;
    private destroyModel;
    private removeTabAction;
    removeTab(id: string, closeAll?: boolean, animate?: boolean, isSaveLayout?: boolean): void;
    moveTab(tab: Tab, nextId?: string): void;
    split(direction: Config.TUILayoutDirection): Wnd;
    private remove;
}
