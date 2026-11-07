/// <reference types="./src/types" />
import { Tab } from "../layout/Tab";
import { Custom } from "../layout/dock/Custom";
import { App } from "../index";
export declare const newCardModel: (options: {
    app: App;
    tab: Tab;
    data: {
        cardType: TCardType;
        id: string;
        title?: string;
        cardsData?: ICardData;
        index?: number;
    };
}) => Custom;
