const path = require("path");

// Protocol/session library, not an HTML editor entry. S07's subsequent adapter
// must bind the native rich-text editor before host editorReady can be enabled.
module.exports = {
    mode: "production",
    entry: "./src/portable/index.ts",
    output: {
        path: path.resolve(__dirname, "stage/build/portable"),
        filename: "session.js",
        library: {name: "WikiPortable", type: "umd"},
        globalObject: "globalThis",
    },
    resolve: {extensions: [".ts", ".js"]},
    module: {rules: [{test: /\.ts$/, use: {loader: "esbuild-loader", options: {target: "es2020"}}}]},
};
