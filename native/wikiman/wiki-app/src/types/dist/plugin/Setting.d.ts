/// <reference types="./src/types" />
import { Dialog } from "../dialog";
export declare class Setting {
    private items;
    private confirmCallback;
    private destroyCallback;
    private width;
    private height;
    dialog: Dialog;
    constructor(options: {
        height?: string;
        width?: string;
        destroyCallback?: () => void;
        confirmCallback?: () => void;
    });
    addItem(options: IPluginSettingOption): void;
    open(name: string): void;
}
