import { Tab } from "../layout/Tab";
interface windowOptions {
    position?: {
        x: number;
        y: number;
    };
    width?: number;
    height?: number;
}
export declare const openNewWindow: (tab: Tab, options?: windowOptions) => void;
export declare const openNewWindowById: (id: string | string[], options?: windowOptions) => Promise<void>;
export declare const openAssetNewWindow: (assetPath: string, options?: windowOptions) => void;
export {};
