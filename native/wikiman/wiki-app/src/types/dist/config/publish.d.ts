/// <reference types="./src/types" />
export declare const publish: {
    element: Element;
    genHTML: () => string;
    bindEvent: () => void;
    _refreshPublish: () => void;
    _savePublish: (reloadAccounts?: boolean) => void;
    _updatePublishConfig: (reloadAccounts: boolean, response: IWebSocketData) => void;
    _renderPublishAuthAccounts: (element: Element, accounts?: Config.IPublishAuthAccount[]) => void;
    _renderPublishAddressList: (element: Element, port: number) => void;
};
