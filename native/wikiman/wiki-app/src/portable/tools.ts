import type {ProtyleBinding} from "./protyle";
import {isPortableWindow} from "./boundary";

/** The portable factory must not expose a legacy tool merely because its
 * underlying DOM handler exists. Unsupported hints are UI-only suppression;
 * this never marks input saved or touches the native source. */
export const PORTABLE_FORMAT_TOOLS=["strong", "em", "u", "s", "mark", "sup", "sub", "kbd", "code", "a", "text", "inline-math", "inline-memo"];

const memoTags = new Set("b strong i em u s del mark sub sup code kbd span br p div pre ul ol li blockquote a h1 h2 h3 h4 h5 h6 table thead tbody tfoot tr th td".split(" "));

/** A memo is inline annotation content. Its preview never loads resources. */
export function sanitizePortableMemo(source: string): string {
    const template = document.createElement("template");
    template.innerHTML = source;
    for (const node of template.content.querySelectorAll("*")) {
        if (node.namespaceURI !== "http://www.w3.org/1999/xhtml" || !memoTags.has(node.localName)) {
            node.remove();
            continue;
        }
        for (const attribute of Array.from(node.attributes)) {
            node.removeAttributeNode(attribute);
        }
    }
    return template.innerHTML;
}
export function blockPortableHint(binding:ProtyleBinding|undefined,element:HTMLElement,notify=false):boolean{
    if(!binding&&!isPortableWindow()){return false;}
    element.classList.add("fn__none");element.replaceChildren();
    if(notify){binding?.reportUnavailableTool();}
    return true;
}
