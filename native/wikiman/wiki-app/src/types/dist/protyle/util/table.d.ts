export declare const getColIndex: (cellElement: HTMLElement) => number;
export declare const setTableAlign: (protyle: IProtyle, cellElements: HTMLElement[], nodeElement: Element, type: string, range: Range) => void;
export declare const insertRow: (protyle: IProtyle, range: Range, cellElement: HTMLElement, nodeElement: Element) => void;
export declare const insertRowAbove: (protyle: IProtyle, range: Range, cellElement: HTMLElement, nodeElement: Element) => void;
export declare const insertColumn: (protyle: IProtyle, nodeElement: Element, cellElement: HTMLElement, type: InsertPosition, range: Range) => void;
export declare const deleteRow: (protyle: IProtyle, range: Range, cellElement: HTMLElement, nodeElement: Element) => void;
export declare const deleteColumn: (protyle: IProtyle, range: Range, nodeElement: Element, cellElement: HTMLElement) => void;
export declare const moveRowToUp: (protyle: IProtyle, range: Range, cellElement: HTMLElement, nodeElement: Element) => void;
export declare const moveRowToDown: (protyle: IProtyle, range: Range, cellElement: HTMLElement, nodeElement: Element) => void;
export declare const moveColumnToLeft: (protyle: IProtyle, range: Range, cellElement: HTMLElement, nodeElement: Element) => void;
export declare const moveColumnToRight: (protyle: IProtyle, range: Range, cellElement: HTMLElement, nodeElement: Element) => void;
export declare const fixTable: (protyle: IProtyle, event: KeyboardEvent, range: Range) => boolean;
export declare const isIncludeCell: (options: {
    tableSelectElement: HTMLElement;
    scrollLeft: number;
    scrollTop: number;
    item: HTMLTableCellElement;
}) => boolean;
export declare const clearTableCell: (protyle: IProtyle, tableBlockElement: HTMLElement) => void;
