/// <reference types="./src/types" />
import { App } from "../index";
export declare const openWechatNotify: (nodeElement: Element) => void;
export declare const openFileWechatNotify: (protyle: IProtyle) => void;
export declare const openFileAttr: (attrs: IObject, focusName?: string, protyle?: IProtyle) => void;
export declare const openAttr: (nodeElement: Element, focusName: string, protyle: IProtyle) => void;
export declare const copySubMenu: (ids: string[], accelerator?: boolean, focusElement?: Element, stdMarkdownId?: string) => ({
    id: string;
    iconHTML: string;
    accelerator: string;
    label: any;
    click: () => void;
} | {
    id: string;
    iconHTML: string;
    label: any;
    click: () => void;
    accelerator?: undefined;
})[];
export declare const exportMd: (id: string) => HTMLElement;
export declare const openMenu: (app: App, src: string, onlyMenu: boolean, showAccelerator: boolean) => ({
    id: string;
    label: any;
    accelerator: any;
    click: () => void;
    icon?: undefined;
} | {
    id: string;
    icon: string;
    label: any;
    accelerator: any;
} | {
    id: string;
    label: any;
    icon: string;
    accelerator?: undefined;
})[];
export declare const renameMenu: (options: {
    path: string;
    notebookId: string;
    name: string;
    type: "notebook" | "file";
}) => HTMLElement;
export declare const movePathToMenu: (paths: string[]) => HTMLElement;
