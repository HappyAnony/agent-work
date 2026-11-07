/// <reference types="./src/types" />
export declare const goSearchRollupCol: (options: {
    target: HTMLElement;
    data: IAV;
    protyle: IProtyle;
    colId: string;
    isRelation: boolean;
}) => void;
export declare const getRollupHTML: (options: {
    data?: IAV;
    cellElements?: HTMLElement[];
    colData?: IAVColumn;
}) => string;
export declare const bindRollupData: (options: {
    protyle: IProtyle;
    data: IAV;
    menuElement: HTMLElement;
}) => void;
