/// <reference types="./src/types" />
import { App } from "../index";
export declare const exportAsset: (src: string) => {
    id: string;
    label: any;
    icon: string;
    click(): Promise<void>;
};
export declare const openEditorTab: (app: App, ids: string[], notebookId?: string, pathString?: string, onlyGetMenus?: boolean) => IMenu[];
export declare const copyPNGByLink: (link: string) => void;
