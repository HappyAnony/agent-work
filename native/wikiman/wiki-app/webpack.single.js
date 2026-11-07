const path = require("path");
const webpack = require("webpack");
const pkg = require("./package.json");
const MiniCssExtractPlugin = require("mini-css-extract-plugin");
const HtmlWebpackPlugin = require("html-webpack-plugin");
const {CleanWebpackPlugin} = require("clean-webpack-plugin");
// const BundleAnalyzerPlugin = require("webpack-bundle-analyzer").BundleAnalyzerPlugin;
const {EsbuildPlugin} = require("esbuild-loader");

module.exports = (env, argv) => {
    return {
        mode: argv.mode || "development",
        watch: argv.mode !== "production",
        devtool: argv.mode !== "production" ? "eval-source-map" : false,
        output: {
            publicPath: "/stage/build/single/",
            filename: "[name].[chunkhash].js",
            path: path.resolve(__dirname, "stage/build/single"),
        },
        entry: {
            "main": "./src/single/index.ts",
        },
        optimization: {
            minimize: true,
            minimizer: [
                new EsbuildPlugin({
                    target: "es6",
                    sourcemap: argv.mode !== "production",
                }),
            ],
        },
        resolve: {
            alias: {
                [path.resolve(__dirname, "src/index")]: path.resolve(__dirname, "src/single/stubs/index"),
                [path.resolve(__dirname, "src/index.ts")]: path.resolve(__dirname, "src/single/stubs/index"),
                [path.resolve(__dirname, "src/layout")]: path.resolve(__dirname, "src/single/stubs/layout"),
            },
            fallback: {
                "path": require.resolve("path-browserify"),
            },
            extensions: [".ts", ".js", ".tpl", ".scss"],
        },
        module: {
            rules: [
                {
                    test: /\.tpl/,
                    include: [
                        path.resolve(__dirname, "src/assets/template/single/index.tpl")],
                    loader: "html-loader",
                    options: {
                        sources: false,
                    },
                },
                {
                    test: /\.ts(x?)$/,
                    include: [path.resolve(__dirname, "src")],
                    use: [
                        {
                            loader: "esbuild-loader",
                            options: {
                                target: "es6",
                                sourcemap: argv.mode !== "production",
                            }
                        },
                        {
                            loader: "ifdef-loader",
                            options: {
                                "ifdef-verbose": false,
                                BROWSER: true,
                                MOBILE: false,
                                SINGLE: true,
                            },
                        },
                    ],
                },
                {
                    test: /\.scss$/,
                    include: [
                        path.resolve(__dirname, "src/assets/scss"),
                    ],
                    use: [
                        MiniCssExtractPlugin.loader,
                        {
                            loader: "css-loader", // translates CSS into CommonJS
                            options: {
                                sourceMap: argv.mode !== "production",
                            },
                        },
                        {
                            loader: "sass-loader", // compiles Sass to CSS
                            options: {
                                sourceMap: argv.mode !== "production",
                            },
                        },
                    ],
                },
                {
                    test: /\.(png|svg)$/,
                    use: [
                        {
                            loader: "file-loader",
                            options: {
                                name: "[name].[ext]",
                                outputPath: "../../",
                            },
                        },
                    ],
                },
            ],
        },
        plugins: [
            // new BundleAnalyzerPlugin(),
            new CleanWebpackPlugin({
                cleanStaleWebpackAssets: false,
                cleanOnceBeforeBuildPatterns: [
                    path.join(__dirname, "stage/build/single")],
            }),
            new webpack.DefinePlugin({
                WIKI_VERSION: JSON.stringify(pkg.version),
                NODE_ENV: JSON.stringify(argv.mode),
            }),
            new MiniCssExtractPlugin({
                filename: "base.[contenthash].css",
            }),
            new HtmlWebpackPlugin({
                inject: "head",
                chunks: ["main"],
                filename: "index.html",
                template: "src/assets/template/single/index.tpl",
            }),
        ],
    };
};
