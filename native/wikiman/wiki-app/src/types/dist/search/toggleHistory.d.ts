import { Protyle } from "../protyle";
export declare const toggleReplaceHistory: (replaceInputElement: HTMLInputElement) => void;
export declare const toggleSearchHistory: (searchElement: Element, config: Config.IUILayoutTabSearchConfig, edit: Protyle) => void;
export declare const toggleAssetHistory: (assetElement: Element) => void;
export declare const saveKeyList: (type: "keys" | "replaceKeys", value: string) => void;
export declare const saveAssetKeyList: (inputElement: HTMLInputElement) => void;
