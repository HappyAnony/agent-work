/// <reference types="./src/types" />
export declare const encodeBase64: (text: string) => string;
export declare const getTextSiyuanFromTextHTML: (html: string) => {
    textSiyuan: string;
    textHtml: string;
};
export declare const isInTauriIframe: () => boolean;
export declare const openByMobile: (uri: string) => void;
export declare const exportByMobile: (uri: string) => void;
export declare const readText: () => string | Promise<string | void>;
export declare const getLocalFiles: () => Promise<string[]>;
export declare const readClipboard: () => Promise<IClipboardData>;
export declare const writeText: (text: string) => void;
export declare const copyPlainText: (text: string) => Promise<void>;
export declare const getEventName: () => "click" | "touchstart";
export declare const isOnlyMeta: (event: KeyboardEvent | MouseEvent) => boolean;
export declare const isNotCtrl: (event: KeyboardEvent | MouseEvent) => boolean;
export declare const isHuawei: () => boolean;
export declare const isDisabledFeature: (feature: string) => boolean;
export declare const isIPhone: () => boolean;
export declare const isSafari: () => boolean;
export declare const isIPad: () => boolean;
export declare const isMac: () => boolean;
export declare const isWin11: () => Promise<boolean>;
export declare const getScreenWidth: () => number;
export declare const isWindows: () => boolean;
export declare const isInAndroid: () => {
    returnDesktop(): void;
    openExternal(url: string): void;
    exportByDefault(url: string): void;
    changeStatusBarColor(color: string, mode: number): void;
    writeClipboard(text: string): void;
    writeHTMLClipboard(text: string, html: string): void;
    writeImageClipboard(uri: string): void;
    readClipboard(): string;
    readHTMLClipboard(): string;
    getBlockURL(): string;
    hideKeyboard(): void;
    print(title: string, html: string): void;
    getScreenWidthPx(): number;
};
export declare const isInIOS: () => {
    openLink: {
        postMessage: (url: string) => void;
    };
    startKernelFast: {
        postMessage: (url: string) => void;
    };
    changeStatusBar: {
        postMessage: (url: string) => void;
    };
    setClipboard: {
        postMessage: (url: string) => void;
    };
    purchase: {
        postMessage: (url: string) => void;
    };
    print: {
        postMessage: (html: string) => void;
    };
};
export declare const isInHarmony: () => {
    openExternal(url: string): void;
    exportByDefault(url: string): void;
    changeStatusBarColor(color: string, mode: number): void;
    writeClipboard(text: string): void;
    writeHTMLClipboard(text: string, html: string): void;
    readClipboard(): string;
    readHTMLClipboard(): string;
    returnDesktop(): void;
    print(title: string, html: string): void;
    getScreenWidthPx(): number;
};
export declare const updateHotkeyAfterTip: (hotkey: string, split?: string) => string;
export declare const updateHotkeyTip: (hotkey: string) => string;
export declare const getLocalStorage: (cb: () => void) => void;
export declare const setStorageVal: (key: string, val: any, cb?: () => void) => void;
export declare const initFocusFix: () => void;
