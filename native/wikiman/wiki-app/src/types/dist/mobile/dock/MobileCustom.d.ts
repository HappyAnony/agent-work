export declare class MobileCustom {
    element: Element;
    data: any;
    type: string;
    init: (custom: MobileCustom) => void;
    destroy: () => void;
    update: () => void;
    constructor(options: {
        element: Element;
        type: string;
        data: any;
        destroy?: () => void;
        update?: () => void;
        init: (custom: MobileCustom) => void;
    });
}
