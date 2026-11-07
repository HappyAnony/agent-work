/// <reference types="./src/types" />
import { App } from "../index";
import { Plugin } from "../plugin";
export declare const reloadSync: (app: App, data: {
    upsertRootIDs: string[];
    removeRootIDs: string[];
}, hideMsg?: boolean, updateReadonly?: boolean, onlyUpdateDoc?: boolean) => void;
export declare const setRefDynamicText: (data: {
    "blockID": string;
    "defBlockID": string;
    "refText": string;
    "rootID": string;
}) => void;
export declare const setDefRefCount: (data: {
    "blockID": string;
    "refCount": number;
    "rootRefCount": number;
    "rootID": string;
}) => void;
export declare const lockScreen: (app: App) => void;
export declare const kernelError: () => void;
export declare const exitSiYuan: () => Promise<void>;
export declare const transactionError: () => void;
export declare const refreshFileTree: (cb?: () => void) => void;
export declare const progressStatus: (data: IWebSocketData) => void;
export declare const progressLoading: (data: IWebSocketData) => void;
export declare const progressBackgroundTask: (tasks: {
    action: string;
}[]) => void;
export declare const bootSync: () => void;
export declare const setTitle: (title: string) => void;
export declare const downloadProgress: (data: {
    id: string;
    percent: number;
}) => void;
export declare const processSync: (data?: IWebSocketData, plugins?: Plugin[]) => void;
