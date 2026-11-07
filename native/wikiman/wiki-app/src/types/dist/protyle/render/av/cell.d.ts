/// <reference types="./src/types" />
export declare const getCellText: (cellElement: HTMLElement | false) => string;
export declare const genCellValueByElement: (colType: TAVCol, cellElement: HTMLElement) => IAVCellValue;
export declare const genCellValue: (colType: TAVCol, value: string | any) => IAVCellValue;
export declare const cellScrollIntoView: (blockElement: HTMLElement, cellElement: Element, onlyHeight?: boolean) => void;
export declare const getTypeByCellElement: (cellElement: Element) => TAVCol;
export declare const popTextCell: (protyle: IProtyle, cellElements: HTMLElement[], type?: TAVCol) => void;
export declare const updateCellsValue: (protyle: IProtyle, nodeElement: HTMLElement, value?: any, cElements?: HTMLElement[], columns?: IAVColumn[], html?: string, getOperations?: boolean) => Promise<{
    doOperations: IOperation[];
    undoOperations: IOperation[];
    text?: undefined;
    json?: undefined;
} | {
    text: string;
    json: IAVCellValue[][];
    doOperations?: undefined;
    undoOperations?: undefined;
}>;
export declare const renderCellAttr: (cellElement: Element, value: IAVCellValue) => void;
export declare const renderCell: (cellValue: IAVCellValue, rowIndex?: number, showIcon?: boolean, type?: TAVView) => string;
export declare const updateHeaderCell: (cellElement: HTMLElement, headerValue: {
    icon?: string;
    name?: string;
    pin?: boolean;
}) => void;
export declare const getPositionByCellElement: (cellElement: HTMLElement) => {
    rowIndex: number;
    celIndex: number;
};
export declare const dragFillCellsValue: (protyle: IProtyle, nodeElement: HTMLElement, originData: {
    [key: string]: IAVCellValue[];
}, originCellIds: string[], activeElement: Element) => void;
export declare const addDragFill: (cellElement: Element) => void;
export declare const cellValueIsEmpty: (value: IAVCellValue) => boolean;
