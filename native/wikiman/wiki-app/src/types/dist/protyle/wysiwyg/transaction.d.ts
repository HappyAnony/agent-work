/// <reference types="./src/types" />
export declare const onTransaction: (protyle: IProtyle, operation: IOperation, isUndo: boolean) => void;
export declare const turnsIntoOneTransaction: (options: {
    protyle: IProtyle;
    selectsElement: Element[];
    type: TTurnIntoOne;
    level?: TTurnIntoOneSub;
}) => void;
export declare const turnsIntoTransaction: (options: {
    protyle: IProtyle;
    selectsElement?: Element[];
    nodeElement?: Element;
    type: TTurnInto;
    level?: number;
    isContinue?: boolean;
    range?: Range;
}) => void;
export declare const turnsOneInto: (options: {
    protyle: IProtyle;
    nodeElement: Element;
    id: string;
    type: string;
    level?: number;
}) => Promise<void>;
export declare const transaction: (protyle: IProtyle, doOperations: IOperation[], undoOperations?: IOperation[]) => void;
export declare const updateTransaction: (protyle: IProtyle, id: string, newHTML: string, html: string) => void;
export declare const updateBatchTransaction: (nodeElements: Element[], protyle: IProtyle, cb: (e: HTMLElement) => void) => void;
