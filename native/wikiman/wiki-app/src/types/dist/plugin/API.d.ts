/// <reference types="./src/types" />
import { Plugin } from "./index";
import { Dialog } from "../dialog";
import { Tab } from "../layout/Tab";
import * as platformUtils from "../protyle/util/compatibility";
import { App } from "../index";
import { Constants } from "../constants";
import { Setting } from "./Setting";
import { Menu } from "./Menu";
import { Protyle } from "../protyle";
import { Model } from "../layout/Model";
export declare const expandDocTree: (options: {
    id: string;
    isSetCurrent?: boolean;
}) => Promise<void>;
export declare const API: {
    adaptHotkey: (hotkey: string) => string;
    confirm: (title: string, text: string, confirm?: (dialog?: Dialog) => void, cancel?: (dialog: Dialog) => void, isDelete?: boolean) => void;
    Constants: typeof Constants;
    showMessage: (message: string, timeout?: number, type?: string, messageId?: string) => string;
    hideMessage: (id?: string) => void;
    fetchPost: (url: string, data?: any, cb?: (response: IWebSocketData) => void, headers?: IObject) => void;
    fetchSyncPost: (url: string, data?: any) => Promise<IWebSocketData>;
    fetchGet: (url: string, cb: (response: string | IObject | IWebSocketData) => void) => void;
    getFrontend: () => "mobile" | "browser-mobile" | "desktop-window" | "desktop" | "browser-desktop";
    getBackend: () => "docker" | "ios" | "android" | "harmony" | "std" | "darwin" | "linux" | "windows";
    getModelByDockType: (type: TDock | string) => any;
    openTab: (options: {
        app: App;
        doc?: {
            id: string;
            action?: TProtyleAction[];
            zoomIn?: boolean;
        };
        pdf?: {
            path: string;
            page?: number;
            id?: string;
        };
        asset?: {
            path: string;
        };
        search?: Config.IUILayoutTabSearchConfig;
        card?: {
            type: TCardType;
            id?: string;
            title?: string;
        };
        custom?: {
            title: string;
            icon: string;
            data?: any;
            id: string;
        };
        position?: "right" | "bottom";
        keepCursor?: boolean;
        removeCurrentTab?: boolean;
        afterOpen?: (model?: Model) => void;
    }) => Promise<Tab>;
    openWindow: (options: {
        position?: IPosition;
        height?: number;
        width?: number;
        tab?: Tab;
        doc?: {
            id: string;
        };
    }) => void;
    openMobileFileById: (app: App, id: string, action?: TProtyleAction[], scrollPosition?: ScrollLogicalPosition) => void;
    lockScreen: (app: App) => void;
    exitSiYuan: () => Promise<void>;
    Protyle: typeof Protyle;
    Plugin: typeof Plugin;
    Dialog: typeof Dialog;
    Menu: typeof Menu;
    Setting: typeof Setting;
    getAllEditor: () => Protyle[];
    getActiveTab: (wndActive?: boolean) => Tab;
    getAllModels: () => IModels;
    getActiveEditor: (wndActive?: boolean) => Protyle;
    platformUtils: typeof platformUtils;
    openSetting: (app: App) => Dialog;
    openAttributePanel: (options: {
        data?: IObject;
        nodeElement?: HTMLElement;
        focusName: "bookmark" | "name" | "alias" | "memo" | "av" | "custom";
        protyle?: IProtyle;
    }) => void;
    saveLayout: (cb: () => void) => void;
    globalCommand: (command: string, app: App) => boolean;
    expandDocTree: (options: {
        id: string;
        isSetCurrent?: boolean;
    }) => Promise<void>;
};
