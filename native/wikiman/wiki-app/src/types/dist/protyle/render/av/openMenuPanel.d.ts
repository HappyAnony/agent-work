/// <reference types="./src/types" />
export declare const openMenuPanel: (options: {
    protyle: IProtyle;
    blockElement: Element;
    type: "select" | "properties" | "config" | "sorts" | "filters" | "edit" | "date" | "asset" | "switcher" | "relation" | "rollup";
    colId?: string;
    editData?: {
        previousID: string;
        colData: IAVColumn;
    };
    cellElements?: HTMLElement[];
    cb?: (avPanelElement: Element) => void;
}) => void;
export declare const getPropertiesHTML: (fields: IAVColumn[]) => string;
