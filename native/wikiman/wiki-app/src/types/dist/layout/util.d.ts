/// <reference types="./src/types" />
import { Layout } from "./index";
import { Wnd } from "./Wnd";
import { Tab } from "./Tab";
import { Model } from "./Model";
import { App } from "../index";
export declare const setPanelFocus: (element: Element, isSaveLayout?: boolean) => void;
export declare const switchWnd: (newWnd: Wnd, targetWnd: Wnd) => void;
export declare const getWndByLayout: (layout: Layout) => Wnd;
export declare const resetLayout: () => void;
export declare const saveLayout: () => void;
export declare const exportLayout: (options: {
    cb: () => void;
    errorExit: boolean;
}) => Promise<void>;
export declare const getAllLayout: () => any;
export declare const JSONToCenter: (app: App, json: Config.TUILayoutItem, layout?: Layout | Wnd | Tab | Model) => void;
export declare const JSONToLayout: (app: App, isStart: boolean) => void;
export declare const layoutToJSON: (layout: Layout | Wnd | Tab | Model, json: any, breakObj?: IObject) => void;
export declare const resizeTopBar: () => void;
export declare const newModelByInitData: (app: App, tab: Tab, json: any) => Model;
export declare const pdfIsLoading: (element: HTMLElement) => boolean;
export declare const getInstanceById: (id: string, layout?: Layout) => Tab | Wnd | Layout;
export declare const addResize: (obj: Layout | Wnd) => void;
export declare const adjustLayout: (layout?: Layout) => void;
export declare const fixWndFlex1: (layout: Layout) => void;
