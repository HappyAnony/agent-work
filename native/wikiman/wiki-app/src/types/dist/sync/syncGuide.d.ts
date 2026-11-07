import { App } from "../index";
export declare const addCloudName: (cloudPanelElement: Element) => void;
export declare const bindSyncCloudListEvent: (cloudPanelElement: Element, cb?: () => void) => void;
export declare const getSyncCloudList: (cloudPanelElement: Element, reload?: boolean, cb?: () => void) => void;
export declare const syncGuide: (app?: App) => void;
export declare const setKey: (isSync: boolean, cb?: () => void) => void;
