/// <reference types="./src/types" />
import { ToolbarItem } from "./ToolbarItem";
export declare class Font extends ToolbarItem {
    element: HTMLElement;
    constructor(protyle: IProtyle, menuItem: IMenuItem);
}
export declare const appearanceMenu: (protyle: IProtyle, nodeElements?: Element[]) => HTMLDivElement;
export declare const fontEvent: (protyle: IProtyle, nodeElements: Element[], type?: string, color?: string) => void;
export declare const setFontStyle: (textElement: HTMLElement, textOption: ITextOption) => void;
export declare const hasSameTextStyle: (currentElement: HTMLElement, sideElement: HTMLElement, textObj?: ITextOption) => boolean;
export declare const getFontNodeElements: (protyle: IProtyle) => Element[];
