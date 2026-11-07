/// <reference types="./src/types" />
import "../assets/scss/export.scss";
declare class Protyle {
    /** 对 graphviz 进行渲染 */
    static graphvizRender: (element: Element, cdn?: string) => void;
    /** 为 element 中的代码块进行高亮渲染 */
    static highlightRender: (element: Element, cdn?: string, zoom?: number) => void;
    /** 对数学公式进行渲染 */
    static mathRender: (element: Element, cdn?: string, maxWidth?: boolean) => void;
    /** 流程图/时序图/甘特图渲染 */
    static mermaidRender: (element: Element, cdn?: string) => void;
    /** flowchart.js 渲染 */
    static flowchartRender: (element: Element, cdn?: string) => void;
    /** 图表渲染 */
    static chartRender: (element: Element, cdn?: string) => void;
    /** 五线谱渲染 */
    static abcRender: (element: Element, cdn?: string) => void;
    /** 脑图渲染 */
    static mindmapRender: (element: Element, cdn?: string) => void;
    /** UML 渲染 */
    static plantumlRender: (element: Element, cdn?: string) => void;
    static avRender: (element: Element, protyle: IProtyle, cb?: (data: IAV) => void, renderAll?: boolean, avData?: IAV) => Promise<void>;
    static htmlRender: (element: Element) => void;
}
export default Protyle;
