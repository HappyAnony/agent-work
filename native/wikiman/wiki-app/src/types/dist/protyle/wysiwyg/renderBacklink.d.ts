export declare const renderBacklink: (protyle: IProtyle, backlinkData: {
    blockPaths: IBreadcrumb[];
    dom: string;
    expand: boolean;
}[]) => void;
export declare const foldPassiveType: (expand: boolean, element: HTMLElement | DocumentFragment) => void;
export declare const loadBreadcrumb: (protyle: IProtyle, element: HTMLElement) => void;
export declare const getBacklinkHeadingMore: (moreElement: HTMLElement) => void;
export declare const genBreadcrumb: (blockPaths: IBreadcrumb[], renderFirst: boolean, parentIndex?: number) => string;
export declare const improveBreadcrumbAppearance: (element: HTMLElement) => void;
