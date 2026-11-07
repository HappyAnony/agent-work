import { App } from "../../index";
export declare const openBacklink: (options: {
    app: App;
    blockId: string;
    rootId?: string;
    title?: string;
    useBlockId?: boolean;
}) => Promise<void>;
export declare const openGraph: (options: {
    app: App;
    blockId: string;
    rootId?: string;
    title?: string;
    useBlockId?: boolean;
}) => Promise<void>;
export declare const openOutline: (options: {
    app: App;
    rootId: string;
    isPreview: boolean;
    title: string;
}) => Promise<void>;
export declare const resetFloatDockSize: () => void;
export declare const toggleDockBar: (useElement: Element) => void;
export declare const clearOBG: () => void;
export declare const selectOpenTab: () => Promise<boolean>;
