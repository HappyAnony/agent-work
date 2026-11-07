/// <reference types="./src/types" />
export declare const fetchPost: (url: string, data?: any, cb?: (response: IWebSocketData) => void, headers?: IObject) => void;
export declare const fetchSyncPost: (url: string, data?: any) => Promise<IWebSocketData>;
export declare const fetchGet: (url: string, cb: (response: IWebSocketData | IObject | string) => void) => void;
