/// <reference types="./src/types" />
import { Tree } from "../../util/Tree";
import { App } from "../../index";
import { Model } from "../../layout/Model";
export declare class MobileOutline extends Model {
    tree: Tree;
    element: HTMLElement;
    blockId: string;
    isPreview: boolean;
    private preFilterExpandIds;
    constructor(options: {
        app: App;
        blockId: string;
        isPreview: boolean;
    });
    setCurrent(nodeElement: HTMLElement): void;
    setCurrentByPreview(nodeElement: Element): void;
    private setCurrentById;
    update(data: IWebSocketData, callbackId?: string): void;
    saveExpendIds(): void;
    /**
     * 应用大纲筛选
     */
    private setFilter;
    /**
     * 获取标题元素的实际标题级别（H1=1, H2=2, 等等）
     * @param element li元素
     * @returns 标题级别（1-6）
     */
    private getHeadingLevel;
    /**
     * 展开到指定标题级别
     * @param targetLevel 目标标题级别，1-6级（H1-H6），6级表示全部展开
     */
    private expandToLevel;
    /**
     * 显示展开层级菜单
     */
    private showExpandLevelMenu;
    /**
     * 切换同层级的所有标题的展开/折叠状态（基于标题级别而不是DOM层级）
     */
    private collapseSameLevel;
    private collapseChildren;
    private onTransaction;
    /**
     * 显示右键菜单
     */
    private showContextMenu;
    private getProtyleAndBlockElement;
    /**
     * 生成标题级别转换菜单项
     */
    private genHeadingTransform;
}
