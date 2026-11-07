/// <reference types="./src/types" />
export declare const renderKanban: (options: {
    blockElement: HTMLElement;
    protyle: IProtyle;
    cb?: (data: IAV) => void;
    renderAll: boolean;
    data?: IAV;
}) => Promise<void>;
