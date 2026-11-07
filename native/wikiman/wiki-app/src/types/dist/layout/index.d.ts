/// <reference types="./src/types" />
import { Wnd } from "./Wnd";
export declare class Layout {
    element: HTMLElement;
    children?: Array<Layout | Wnd>;
    parent?: Layout;
    direction: Config.TUILayoutDirection;
    type?: Config.TUILayoutType;
    id?: string;
    resize?: Config.TUILayoutDirection;
    size?: string;
    constructor(options?: ILayoutOptions);
    addLayout(child: Layout, id?: string): void;
    addWnd(child: Wnd, id?: string): void;
}
