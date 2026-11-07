/// <reference types="./src/types" />
import { Dialog } from "../dialog";
import { Protyle } from "../protyle";
import { App } from "../index";
export declare const genCardHTML: (options: {
    id: string;
    cardType: TCardType;
    cardsData: ICardData;
    isTab: boolean;
}) => string;
export declare const bindCardEvent: (options: {
    app: App;
    element: Element;
    title?: string;
    cardsData: ICardData;
    cardType: TCardType;
    id?: string;
    dialog?: Dialog;
    index?: number;
}) => Promise<Protyle>;
export declare const openCard: (app: App) => void;
export declare const openCardByData: (app: App, cardsData: ICardData, cardType: TCardType, id?: string, title?: string) => Promise<void>;
