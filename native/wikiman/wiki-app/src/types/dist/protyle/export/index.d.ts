/// <reference types="./src/types" />
export declare const saveExport: (option: IExportOptions) => void;
export declare const onExport: (data: IWebSocketData, filePath: string, servePath: string, exportOption: IExportOptions, msgId?: string) => Promise<string>;
