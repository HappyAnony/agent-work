/// <reference types="./src/types" />
export declare const openSearchAsset: (element: HTMLElement, isStick: boolean) => void;
export declare const assetInputEvent: (element: Element, localSearch?: ISearchAssetOption, page?: number) => void;
export declare const renderPreview: (element: Element, id: string, query: string, queryMethod: number) => void;
export declare const renderNextAssetMark: (element: Element) => void;
export declare const assetMethodMenu: (target: HTMLElement, cb: () => void) => void;
export declare const assetFilterMenu: (assetsElement: Element) => void;
export declare const assetMoreMenu: (target: Element, element: Element, cb: () => void) => void;
