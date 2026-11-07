/// <reference types="./src/types" />
import { Layout } from "../index";
import { Model } from "../Model";
import { App } from "../../index";
export declare class Dock {
    element: HTMLElement;
    layout: Layout;
    private position;
    private app;
    resizeElement: HTMLElement;
    pin: boolean;
    data: {
        [key in TDock | string]?: Model | boolean;
    };
    private hideResizeTimeout;
    constructor(options: {
        app: App;
        data: {
            pin: boolean;
            data: Config.IUILayoutDockTab[][];
        };
        position: TDockPosition;
    });
    togglePin(): void;
    resetDockPosition(show: boolean): void;
    showDock(reset?: boolean): void;
    hideDock(reset?: boolean): void;
    toggleModel(type: TDock | string, show?: boolean, close?: boolean, hide?: boolean, isSaveLayout?: boolean): void;
    add(index: number, sourceElement: Element, previousType?: string): void;
    remove(key: TDock | string): void;
    setSize(): void;
    private getMaxSize;
    genButton(data: Config.IUILayoutDockTab[], index: number, tabIndex?: number): void;
}
