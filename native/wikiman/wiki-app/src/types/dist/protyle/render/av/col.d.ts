/// <reference types="./src/types" />
import { Menu } from "../../../plugin/Menu";
export declare const getColId: (element: Element, viewType: TAVView) => string;
export declare const duplicateCol: (options: {
    protyle: IProtyle;
    colId: string;
    viewID: string;
    blockElement: Element;
    data: IAV;
}) => void;
export declare const getEditHTML: (options: {
    protyle: IProtyle;
    colId: string;
    data: IAV;
    isCustomAttr: boolean;
}) => string;
export declare const bindEditEvent: (options: {
    protyle: IProtyle;
    data: IAV;
    blockID: string;
    menuElement: HTMLElement;
    isCustomAttr: boolean;
}) => void;
export declare const getColNameByType: (type: TAVCol) => any;
export declare const getColIconByType: (type: TAVCol) => "iconLink" | "iconCheck" | "iconOrderedList" | "iconList" | "iconListItem" | "iconMath" | "iconImage" | "iconOpen" | "iconAlignLeft" | "iconKey" | "iconNumber" | "iconSearch" | "iconCalendar" | "iconClock" | "iconEmail" | "iconPhone";
export declare const showColMenu: (protyle: IProtyle, blockElement: Element, cellElement: HTMLElement) => void;
export declare const removeCol: (options: {
    protyle: IProtyle;
    fields: IAVColumn[];
    avID: string;
    blockID: string;
    isCustomAttr: boolean;
    menuElement: HTMLElement;
    blockElement: Element;
    avPanelElement: Element;
    tabRect: DOMRect;
    isTwoWay: boolean;
}) => void;
export declare const addCol: (protyle: IProtyle, blockElement: Element, previousID?: string) => Menu;
