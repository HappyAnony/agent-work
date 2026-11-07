import { Tab } from "../layout/Tab";
import { App } from "../index";
import { Layout } from "../layout";
import { Wnd } from "../layout/Wnd";
export declare const initTabMenu: (app: App, tab: Tab) => import("./Menu").Menu;
export declare const unsplitWnd: (target: Wnd | Layout, layout: Layout, onlyWnd: boolean) => void;
