import { Model } from "../layout/Model";
import { Tab } from "../layout/Tab";
import { App } from "../index";
export declare class Asset extends Model {
    path: string;
    element: HTMLElement;
    private pdfId;
    private pdfPage;
    pdfObject: any;
    constructor(options: {
        app: App;
        tab: Tab;
        path: string;
        page?: number | string;
    });
    private getPdfId;
    goToPage(pdfId: string | number): void;
    private render;
}
