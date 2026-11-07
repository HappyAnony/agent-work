export declare class Upload {
    element: HTMLElement;
    isUploading: boolean;
    constructor();
}
export declare const uploadLocalFiles: (files: string[], protyle: IProtyle, isUpload: boolean) => void;
export declare const uploadFiles: (protyle: IProtyle, files: FileList | DataTransferItemList | File[], element?: HTMLInputElement, successCB?: (res: string) => void) => void;
