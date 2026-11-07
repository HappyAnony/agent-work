/// <reference types="./src/types" />
import { Tab } from "./Tab";
import { App } from "../index";
export declare const getActiveTab: (wndActive?: boolean) => Tab;
export declare const switchTabByIndex: (index: number) => void;
export declare const resizeTabs: (isSaveLayout?: boolean) => void;
export declare const getDockByType: (type: TDock | string) => import("./dock").Dock;
export declare const newCenterEmptyTab: (app: App) => Tab;
export declare const copyTab: (app: App, tab: Tab) => Tab;
export declare const closeTabByType: (tab: Tab, type: "closeOthers" | "closeAll" | "other", tabs?: Tab[]) => Promise<void>;
