/// <reference types="./src/types" />
interface IIds {
    groupId: string;
    fieldId: string;
}
interface ITableOptions {
    protyle: IProtyle;
    blockElement: HTMLElement;
    cb: (data: IAV) => void;
    data: IAV;
    renderAll: boolean;
    resetData: {
        alignSelf: string;
        selectItemIds: IIds[];
        editIds: IIds[];
        isSearching: boolean;
        pageSizes: {
            [key: string]: string;
        };
        query: string;
        oldOffset: number;
        left?: number;
    };
}
export declare const afterRenderGallery: (options: ITableOptions) => void;
export declare const renderGallery: (options: {
    blockElement: HTMLElement;
    protyle: IProtyle;
    cb?: (data: IAV) => void;
    renderAll: boolean;
    data?: IAV;
}) => Promise<void>;
export {};
