/// <reference types="./src/types" />
import { Protyle } from "../protyle";
export declare class Dialog {
    private destroyCallback;
    element: HTMLElement;
    private id;
    private disableClose;
    editors: {
        [key: string]: Protyle;
    };
    data: any;
    constructor(options: {
        positionId?: string;
        title?: string;
        transparent?: boolean;
        content: string;
        width?: string;
        height?: string;
        destroyCallback?: (options?: IObject) => void;
        disableClose?: boolean;
        hideCloseIcon?: boolean;
        disableAnimation?: boolean;
        resizeCallback?: (type: string) => void;
        containerClassName?: string;
    });
    destroy(options?: IObject): void;
    bindInput(inputElement: HTMLInputElement | HTMLTextAreaElement, enterEvent?: () => void, bindEnter?: boolean): void;
}
