/// <reference types="./src/types" />
import { Layout } from "./index";
import { Tab } from "./Tab";
import { Protyle } from "../protyle";
import { Wnd } from "./Wnd";
export declare const getAllEditor: () => Protyle[];
export declare const getAllModels: () => IModels;
export declare const getAllWnds: (layout: Layout, wnds: Wnd[]) => void;
export declare const getAllTabs: () => Tab[];
export declare const getAllDocks: () => Config.IUILayoutDockTab[];
