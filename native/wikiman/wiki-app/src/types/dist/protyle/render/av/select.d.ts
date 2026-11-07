/// <reference types="./src/types" />
export declare const removeCellOption: (protyle: IProtyle, cellElements: HTMLElement[], target: HTMLElement, blockElement: Element) => void;
export declare const setColOption: (protyle: IProtyle, data: IAV, target: HTMLElement, blockElement: Element, isCustomAttr: boolean, cellElements?: HTMLElement[]) => void;
export declare const bindSelectEvent: (protyle: IProtyle, data: IAV, menuElement: HTMLElement, cellElements: HTMLElement[], blockElement: Element) => void;
export declare const addColOptionOrCell: (protyle: IProtyle, data: IAV, cellElements: HTMLElement[], currentElement: HTMLElement, menuElement: HTMLElement, blockElement: Element) => void;
export declare const getSelectHTML: (fields: IAVColumn[], cellElements: HTMLElement[], init: boolean, blockElement: Element) => string;
export declare const mergeAddOption: (column: IAVColumn, cellValue: IAVCellValue, avID: string) => {
    doOperations: IOperation[];
    undoOperations: IOperation[];
};
