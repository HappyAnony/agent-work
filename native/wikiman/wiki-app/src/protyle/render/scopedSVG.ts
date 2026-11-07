import {Constants} from "../../constants";
import {addScript} from "../util/addScript";
import {hasClosestByClassName} from "../util/hasClosest";
import {genIconHTML} from "./util";
import {mindmapOptions} from "./mindmapRender";
import {renderScopedPlantUML} from "./scopedPlantUML";

const diagramSelector = '.render-node:is([data-subtype="mermaid"],[data-subtype="graphviz"],[data-subtype="abc"],[data-subtype="flowchart"],[data-subtype="echarts"],[data-subtype="mindmap"],[data-subtype="plantuml"])';
export const scopedDiagramSelector = `${diagramSelector}, .render-node[data-type="NodeHTMLBlock"]`;
export const scopedRenderInteraction = "[data-portable-render-interaction]";
declare const flowchart: {parse(source: string): {drawSVG(target: Element): void}};
const diagramLabels: Record<string,string> = {mermaid:"Mermaid",graphviz:"Graphviz",abc:"ABC",flowchart:"Flowchart",echarts:"ECharts",mindmap:"Mindmap",plantuml:"PlantUML"};
const renderedSVG = new WeakMap<Element, Blob>();
let queue: Promise<void> = Promise.resolve();

export function downloadScopedPlantUML(diagram: Element): boolean {
    const blob = renderedSVG.get(diagram.querySelector('img[alt="PlantUML"]'));
    if (!blob) { return false; }
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "plantuml.svg";
    anchor.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    return true;
}

function chartOptions(source: string) {
    const options: unknown = JSON.parse(source, (key, value:unknown) => {
        if (["__proto__","prototype","constructor","image","link","sublink","dataView","appendTo","appendToBody"].includes(key) ||
            key === "type" && value === "image" ||
            typeof value === "string" && /(?:image:\/\/|\b(?:https?|file|data|blob):|^\/\/)/i.test(value)) {
            throw new Error("External chart resources are unavailable");
        }
        return value;
    });
    if (!options || typeof options !== "object" || Array.isArray(options)) {
        throw new Error("Invalid chart options");
    }
    return options;
}

function passiveSVG(source: string): SVGElement {
    const parsed = new DOMParser().parseFromString(source, "image/svg+xml");
    const svg = parsed.documentElement as unknown as SVGElement;
    if (svg.localName !== "svg" || svg.namespaceURI !== "http://www.w3.org/2000/svg") { throw new Error("Invalid SVG"); }
    const active = new Set(["script", "foreignobject", "image", "feimage", "iframe", "object", "embed",
        "animate", "animatemotion", "animatetransform", "set"]);
    const externalStyle = (value:string) => /@import|@font-face|[\\]/i.test(value) ||
        /url\s*\(/i.test(value.replace(/url\(\s*["']?#[A-Za-z0-9_:-]+["']?\s*\)/gi,""));
    for (const node of [svg, ...svg.querySelectorAll("*")]) {
        const tag = node.localName.toLowerCase();
        if (node.namespaceURI !== svg.namespaceURI || active.has(tag) ||
            tag === "style" && externalStyle(node.textContent)) { node.remove(); continue; }
        for (const attr of Array.from(node.attributes)) {
            const key = attr.localName.toLowerCase();
            if (/^on/i.test(key) || ["src", "srcset", "base", "target"].includes(key) ||
                key === "href" && (tag === "a" || !/^#[A-Za-z0-9_:-]+$/.test(attr.value)) ||
                externalStyle(attr.value)) { node.removeAttributeNode(attr); }
        }
    }
    return svg;
}

const charts = new Map<HTMLElement, ResizeObserver>();
const chartRemoval = new MutationObserver(() => {
    for (const [target, resize] of charts) {
        if (target.isConnected) { continue; }
        resize.disconnect();
        window.echarts.dispose(target);
        charts.delete(target);
    }
    if (!charts.size) { chartRemoval.disconnect(); }
});

function renderInteractiveChart(output: HTMLElement, options: ReturnType<typeof mindmapOptions> | object, label: string) {
    output.dataset.portableRenderInteraction = "";
    output.setAttribute("role","img");
    output.setAttribute("aria-label",label);
    output.style.height="420px";
    const chart=window.echarts.init(output,window.siyuan.config.appearance.mode===1?"dark":undefined,
        {renderer:"canvas",width:output.clientWidth || 755,height:420});
    try {
        // Canvas and rich-text tooltips keep all source strings out of live HTML.
        chart.setOption({...options,animation:false,tooltip:{...("tooltip" in options ? options.tooltip : {} ) as object,renderMode:"richText"}});
    } catch (error) {
        window.echarts.dispose(output);
        throw error;
    }
    const resize=new ResizeObserver(() => { if (output.isConnected && output.clientWidth) { chart.resize(); } });
    resize.observe(output);
    if (!charts.size) { chartRemoval.observe(document.body,{childList:true,subtree:true}); }
    charts.set(output,resize);
}

const htmlTags = new Set("div span p h1 h2 h3 h4 h5 h6 strong b em i u s mark small sub sup br hr pre code kbd blockquote ul ol li dl dt dd table thead tbody tfoot tr th td caption section article details summary".split(" "));
function renderScopedHTML(root: Element) {
    const selector = '.render-node[data-type="NodeHTMLBlock"]';
    const blocks = root.matches(selector) ? [root] : Array.from(root.querySelectorAll(selector));
    for (const block of blocks) {
        const holder = block.querySelector("protyle-html");
        if (!holder) { continue; }
        const source = Lute.UnEscapeHTMLStr(holder.getAttribute("data-content") || "");
        const preview = document.createElement("div");
        preview.dataset.portableRenderInteraction = "";
        preview.contentEditable = "false";
        if (source.length > 20000) { preview.textContent = "HTML preview is too large"; }
        else {
            const template = document.createElement("template");
            template.innerHTML = source;
            for (const node of template.content.querySelectorAll("*")) {
                if (node.namespaceURI !== "http://www.w3.org/1999/xhtml" || !htmlTags.has(node.localName)) { node.remove(); continue; }
                for (const attr of Array.from(node.attributes)) {
                    if (!(attr.name === "open" && node.localName === "details")) { node.removeAttributeNode(attr); }
                }
            }
            preview.append(template.content);
        }
        holder.replaceChildren(preview);
        block.querySelector(".protyle-action__edit")?.setAttribute("aria-label",window.siyuan.languages.edit);
    }
}

export function renderScopedDiagrams(root: Element) {
    renderScopedHTML(root);
    const diagrams = root.matches(diagramSelector) ? [root] : Array.from(root.querySelectorAll(diagramSelector));
    for (const diagram of diagrams.slice(0,64)) {
        if (diagram.getAttribute("data-render") === "true") { continue; }
        const value=diagram.getAttribute("data-content") || "";
        const subtype=diagram.getAttribute("data-subtype");
        if (!diagram.firstElementChild?.classList.contains("protyle-icons")) {
            const editable=hasClosestByClassName(diagram,"protyle-wysiwyg",true)&&!diagram.closest(".protyle-wysiwyg__embed");
            diagram.insertAdjacentHTML("afterbegin",editable?genIconHTML():'<div class="protyle-icons"></div>');
        }
        const container=diagram.firstElementChild?.nextElementSibling;
        if (!container) { continue; }
        const output=document.createElement("div");
        output.contentEditable="false";
        output.setAttribute("aria-busy","true");
        container.replaceChildren(output);
        diagram.setAttribute("data-render","true");
        queue=queue.then(async()=>{
            const stillCurrent=()=>output.isConnected&&diagram.getAttribute("data-content")===value;
            if(!stillCurrent()){return;}
            try {
                // Reading the attribute already decodes the BlockDOM HTML.
                const source=value;
                if(!source.trim()){return;}
                if(source.length>20000){throw new Error("Diagram source is too large");}
                let rendered:string;
                if(subtype==="mermaid"){
                    if(/%%\s*\{|^\s*---(?:\r?\n)/.test(source)){throw new Error("Diagram configuration directives are unavailable");}
                    await addScript(Constants.PROTYLE_CDN+"/js/mermaid/mermaid.min.js?v=11.12.0","protyleMermaidScript");
                    if(!stillCurrent()){return;}
                    window.mermaid.initialize({securityLevel:"strict",htmlLabels:false,startOnLoad:false,maxTextSize:20000,maxEdges:200,
                        theme:window.siyuan.config.appearance.mode===1?"dark":"default"});
                    rendered=(await window.mermaid.render("mermaid"+Lute.NewNodeID(),source)).svg;
                }else if(subtype==="plantuml"){
                    rendered=await renderScopedPlantUML(source,window.siyuan.config.appearance.mode===1);
                }else if(subtype==="graphviz"){
                    await addScript(Constants.PROTYLE_CDN+"/js/graphviz/viz.js?v=3.11.0","protyleGraphVizScript");
                    if(!stillCurrent()){return;}
                    rendered=(await Viz.instance()).renderSVGElement(source).outerHTML;
                }else{
                    const detached=document.createElement("div");
                    if(subtype==="abc"){
                        await addScript(Constants.PROTYLE_CDN+"/js/abcjs/abcjs-basic-min.js?v=6.5.0","protyleAbcjsScript");
                        if(!stillCurrent()){return;}
                        window.ABCJS.renderAbc(detached,source,{responsive:"resize"});
                    }else if(subtype==="flowchart"){
                        if(/\b(?:__proto__|prototype|constructor)\b/.test(source)){throw new Error("Invalid flowchart symbol");}
                        await addScript(Constants.PROTYLE_CDN+"/js/flowchart.js/flowchart.min.js?v=1.18.0","protyleFlowchartScript");
                        if(!stillCurrent()){return;}
                        flowchart.parse(source).drawSVG(detached);
                    }else{
                        const options=subtype==="mindmap"?mindmapOptions(source):chartOptions(source);
                        await addScript(Constants.PROTYLE_CDN+"/js/echarts/echarts.min.js?v=5.3.2","protyleEchartsScript");
                        if(!stillCurrent()){return;}
                        renderInteractiveChart(output,options,diagramLabels[subtype]);
                        return;
                    }
                    if(rendered===undefined){
                        const svg=detached.querySelector("svg");
                        if(!svg){throw new Error("Invalid diagram");}
                        rendered=new XMLSerializer().serializeToString(svg);
                    }
                }
                if(stillCurrent()){
                    const svg=passiveSVG(rendered);
                    const blob = new Blob([new XMLSerializer().serializeToString(svg)], {type:"image/svg+xml"});
                    const url=URL.createObjectURL(blob);
                    const image=document.createElement("img");
                    if (subtype === "plantuml") { renderedSVG.set(image, blob); }
                    image.alt=diagramLabels[subtype];
                    image.addEventListener("load",()=>URL.revokeObjectURL(url),{once:true});
                    image.addEventListener("error",()=>URL.revokeObjectURL(url),{once:true});
                    image.src=url;
                    output.replaceChildren(image);
                }
            }catch(error){
                if(stillCurrent()){
                    output.classList.add("ft__error");
                    output.textContent=error instanceof Error?error.message:window.siyuan.languages.operationFailed;
                }
            }finally{output.removeAttribute("aria-busy");}
        });
    }
}
