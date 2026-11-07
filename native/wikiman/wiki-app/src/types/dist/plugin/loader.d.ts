/// <reference types="./src/types" />
import { App } from "../index";
import { Plugin } from "./index";
export declare const loadPlugins: (app: App, names?: string[]) => Promise<void>;
export declare const loadPlugin: (app: App, item: IPluginData) => Promise<Plugin>;
export declare const afterLoadPlugin: (plugin: Plugin) => void;
export declare const reloadPlugin: (app: App, data?: {
    upsertCodePlugins?: string[];
    upsertDataPlugins?: string[];
    removePlugins?: string[];
}) => Promise<void>;
