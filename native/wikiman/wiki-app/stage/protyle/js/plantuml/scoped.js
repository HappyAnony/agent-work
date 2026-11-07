// This frame has an opaque origin and a policy that denies network connections.
const bundledLibraries = new Set(["emoji.js", "openiconic.js", "themes.js"]);
window.PLANTUML_STDLIB_LOADER = (file, done, fail) => {
    if (!bundledLibraries.has(file)) {
        fail("This PlantUML library is unavailable offline");
        return true;
    }
    const script = document.createElement("script");
    script.src = file;
    script.onload = done;
    script.onerror = () => fail("Could not load the bundled PlantUML library");
    document.head.append(script);
    return true;
};

const parentOrigin = new URL(location.href).origin;
let busy = false;
window.addEventListener("message", async event => {
    if (event.source !== parent || event.origin !== parentOrigin || busy || event.data?.type !== "render-plantuml") {
        return;
    }
    busy = true;
    const {source, dark} = event.data;
    const finish = result => {
        busy = false;
        parent.postMessage({type:"plantuml-result", ...result}, parentOrigin);
    };
    if (typeof source !== "string" || source.length > 20000 || typeof dark !== "boolean") {
        finish({error:"Invalid PlantUML source"});
        return;
    }
    try {
        const {renderToString} = await import(new URL("plantuml.js", location.href).href);
        renderToString(source.split(/\r?\n/), svg => finish({svg}),
            () => finish({error:"PlantUML could not render this diagram"}), {dark});
    } catch {
        finish({error:"The bundled PlantUML renderer could not start"});
    }
});
parent.postMessage({type:"plantuml-ready"}, parentOrigin);
