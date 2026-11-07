/// <reference types="./src/types" />
import { App } from "../index";
export declare const initNavigationMenu: (app: App, liElement: HTMLElement) => import("./Menu").Menu;
export declare const initFileMenu: (app: App, notebookId: string, pathString: string, liElement: Element) => import("./Menu").Menu;
export declare const genImportMenu: (notebookId: string, pathString: string) => void;
export declare const sortMenu: (type: "notebooks" | "notebook", sortMode: number, clickEvent: (sort: number) => void) => IMenu[];
