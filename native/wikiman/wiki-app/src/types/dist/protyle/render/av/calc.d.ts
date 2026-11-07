/// <reference types="./src/types" />
export declare const openCalcMenu: (protyle: IProtyle, calcElement: HTMLElement, panelData?: {
    data: IAV;
    colId: string;
    blockID: string;
}, x?: number) => Promise<void>;
export declare const getCalcValue: (column: IAVColumn) => string;
export declare const getNameByOperator: (operator: string, isRollup: boolean) => any;
