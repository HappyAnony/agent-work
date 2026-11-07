/// <reference types="./src/types" />
export declare const bindAssetEvent: (options: {
    protyle: IProtyle;
    menuElement: HTMLElement;
    cellElements: HTMLElement[];
    blockElement: Element;
}) => void;
export declare const getAssetHTML: (cellElements: HTMLElement[]) => string;
export declare const updateAssetCell: (options: {
    protyle: IProtyle;
    cellElements: HTMLElement[];
    replaceValue?: IAVCellAssetValue[];
    addValue?: IAVCellAssetValue[];
    updateValue?: {
        index: number;
        value: IAVCellAssetValue;
    };
    removeIndex?: number;
    blockElement: Element;
}) => void;
export declare const editAssetItem: (options: {
    protyle: IProtyle;
    cellElements: HTMLElement[];
    blockElement: Element;
    content: string;
    type: "image" | "file";
    name: string;
    index: number;
    rect: DOMRect;
}) => void;
export declare const addAssetLink: (protyle: IProtyle, cellElements: HTMLElement[], target: HTMLElement, blockElement: Element) => void;
export declare const dragUpload: (files: string[], protyle: IProtyle, cellElement: HTMLElement) => void;
