/// <reference types="./src/types" />
import { App } from "../index";
export declare const bazaar: {
    element: Element;
    genHTML(): string;
    _genCardHTML(item: IBazaarItem, bazaarType: TBazaarType): string;
    _genUpdateItemHTML(item: IBazaarItem, bazaarType: TBazaarType): string;
    _getUpdate(): void;
    _genMyHTML(bazaarType: TBazaarType, app: App, updateUpdate?: boolean): void;
    _data: {
        themes: IBazaarItem[];
        templates: IBazaarItem[];
        icons: IBazaarItem[];
        widgets: IBazaarItem[];
        plugins: IBazaarItem[];
        downloaded: IBazaarItem[];
        update: {
            themes: IBazaarItem[];
            templates: IBazaarItem[];
            icons: IBazaarItem[];
            widgets: IBazaarItem[];
            plugins: IBazaarItem[];
        };
    };
    _renderReadme(bazaarType: TBazaarType, data: IBazaarItem, downloaded: boolean): void;
    bindEvent(app: App): void;
    _onBazaar(response: IWebSocketData, bazaarType: TBazaarType): void;
};
