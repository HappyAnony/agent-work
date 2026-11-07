export declare const initUI: (protyle: IProtyle) => void;
export declare const addLoading: (protyle: IProtyle, msg?: string) => void;
export declare const removeLoading: (protyle: IProtyle) => void;
export declare const setPadding: (protyle: IProtyle) => {
    width: number;
    padding: number;
} | {
    width: number;
    padding?: undefined;
};
export declare const getPadding: (protyle: IProtyle) => {
    left: number;
    right: number;
    bottom: number;
    top: number;
};
