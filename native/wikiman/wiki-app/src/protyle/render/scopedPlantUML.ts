export function renderScopedPlantUML(source: string, dark: boolean): Promise<string> {
    if (/^\s*!\s*(?:include\w*|import)\b/im.test(source)) {
        throw new Error("PlantUML includes are unavailable offline");
    }
    return new Promise((resolve, reject) => {
        const frame = document.createElement("iframe");
        frame.sandbox.add("allow-scripts");
        frame.hidden = true;
        frame.title = "Local PlantUML renderer";
        frame.src = "/stage/protyle/js/plantuml/scoped.html";
        const timeout = window.setTimeout(() => finish(undefined, "PlantUML rendering timed out"), 10000);
        let requested = false;
        const receive = (event: MessageEvent) => {
            if (event.origin !== "null" || event.source !== frame.contentWindow) { return; }
            if (event.data?.type === "plantuml-ready" && !requested) {
                requested = true;
                // The renderer is deliberately sandboxed to an opaque origin.
                frame.contentWindow.postMessage({type:"render-plantuml", source, dark}, "*");
            } else if (event.data?.type === "plantuml-result" && requested) {
                const {svg, error} = event.data;
                if (typeof svg === "string" && svg.length <= 4 * 1024 * 1024) {
                    finish(svg);
                } else {
                    finish(undefined, typeof error === "string" ? error : "Invalid PlantUML output");
                }
            }
        };
        function finish(svg?: string, error?: string) {
            clearTimeout(timeout);
            window.removeEventListener("message", receive);
            frame.remove();
            if (error) { reject(new Error(error)); }
            else { resolve(svg); }
        }
        window.addEventListener("message", receive);
        document.body.append(frame);
    });
}
