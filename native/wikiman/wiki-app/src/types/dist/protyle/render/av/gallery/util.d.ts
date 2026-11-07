/// <reference types="./src/types" />
export declare const setGalleryCover: (options: {
    view: IAVGallery;
    nodeElement: Element;
    protyle: IProtyle;
    target: HTMLElement;
}) => void;
export declare const setGallerySize: (options: {
    view: IAVGallery;
    nodeElement: Element;
    protyle: IProtyle;
    target: HTMLElement;
}) => void;
export declare const getCardAspectRatio: (ratio: number) => "16:9" | "9:16" | "4:3" | "3:4" | "3:2" | "2:3" | "1:1";
export declare const setGalleryRatio: (options: {
    view: IAVGallery;
    nodeElement: Element;
    protyle: IProtyle;
    target: HTMLElement;
}) => void;
export declare const openGalleryItemMenu: (options: {
    target: HTMLElement;
    protyle: IProtyle;
    position: {
        x: number;
        y: number;
    };
}) => void;
export declare const editGalleryItem: (target: Element) => void;
