const path=require("path");
const fs=require("fs");
const single=require("./webpack.single");
const HtmlWebpackPlugin=require("html-webpack-plugin");
module.exports=(env,argv)=>{
    const config=single(env,{...argv,mode:"production"});
    config.watch=false;
    const subscription=path.resolve(__dirname,"src/util/needSubscribe");
    config.resolve.alias[subscription+"$"]=path.resolve(__dirname,"src/portable/localCapabilities.ts");
    config.resolve.alias[subscription+".ts$"]=path.resolve(__dirname,"src/portable/localCapabilities.ts");
    // Keep the single-view layout stubs while using the original scoped Model.
    // The broad single-view alias otherwise silently discards its subscriptions.
    const layout=path.resolve(__dirname,"src/layout");
    const layoutStubs=config.resolve.alias[layout];
    delete config.resolve.alias[layout];
    for (const file of fs.readdirSync(layoutStubs,{recursive:true})) {
        if (!file.endsWith(".ts") || ["Model.ts", "Tab.ts", "dock/Files.ts", "dock/Backlink.ts", "dock/Graph.ts", "dock/Bookmark.ts", "dock/Tag.ts", "dock/Outline.ts"].includes(file)) { continue; }
        const source=path.join(layout,file),stub=path.join(layoutStubs,file);
        config.resolve.alias[source+"$"]=stub;
        config.resolve.alias[source.slice(0,-3)+"$"]=stub;
    }
    config.entry={protyle:"./src/portable/mount.ts",boot:"./src/portable/boot.ts"};
    config.output={...config.output,publicPath:"/stage/build/portable/",path:path.resolve(__dirname,"stage/build/portable"),filename:"[name].js",library:{name:"WikiPortableProtyle",type:"umd"},globalObject:"globalThis"};
    config.plugins=config.plugins.filter(plugin=>!["HtmlWebpackPlugin","CleanWebpackPlugin"].includes(plugin.constructor.name));
    config.plugins.push(new HtmlWebpackPlugin({chunks:["boot"],filename:"index.html",scriptLoading:"defer",templateContent:'<!doctype html><html><head><meta charset="UTF-8"><title>Wiki editor</title><link rel="stylesheet" href="/appearance/themes/daylight/theme.css"><script src="/stage/protyle/js/lute/lute.min.js"></script><script type="module" src="/stage/protyle/js/pdf/pdf.min.mjs?v=4.7.85"></script></head><body><div id="editor" style="height:100vh"></div></body></html>'}));
    return config;
};
