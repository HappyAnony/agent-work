import {Constants} from "../../constants";
import {addScript} from "../util/addScript";
import {fetchPost} from "../../util/fetch";
import {scopedUIAssetURL} from "../../portable/providers";
import type {ProtyleBinding} from "../../portable/protyle";

export const previewImages = (srcList: string[], currentSrc?: string, active = () => true) => {
    addScript(`${Constants.PROTYLE_CDN}/js/viewerjs/viewer.js?v=1.11.7`, "protyleViewerScript").then(() => {
        if (!active()) { return; }
        const imagesElement = document.createElement("ul");
        let initialViewIndex = -1;
        srcList.filter(Boolean).forEach((item, index) => {
            const row = document.createElement("li");
            const image = document.createElement("img");
            image.src = item;
            row.append(image);
            imagesElement.append(row);
            if (currentSrc && initialViewIndex === -1 && (currentSrc.endsWith(encodeURI(item)) || currentSrc.endsWith(item))) {
                initialViewIndex = index;
            }
        });
        if (!imagesElement.childElementCount) { return; }
        window.siyuan.viewer?.destroy();
        window.siyuan.viewer = new Viewer(imagesElement, {
            initialViewIndex: currentSrc ? initialViewIndex : 0,
            title: [1, (image: HTMLImageElement, imageData: IObject) => {
                let name = image.alt;
                if (!name) {
                    name = image.src.substring(image.src.lastIndexOf("/") + 1);
                }
                name = name.substring(0, name.lastIndexOf(".")).replace(/-\d{14}-\w{7}$/, "");
                return `${name} [${imageData.naturalWidth} × ${imageData.naturalHeight}]`;
            }],
            button: false,
            transition: false,
            hidden: function () {
                window.siyuan.viewer.destroy();
            },
            toolbar: {
                zoomIn: true,
                zoomOut: true,
                oneToOne: true,
                reset: true,
                prev: true,
                play: true,
                next: true,
                rotateLeft: true,
                rotateRight: true,
                flipHorizontal: true,
                flipVertical: true,
                close: function () {
                    window.siyuan.viewer.destroy();
                },
            },
        });
        window.siyuan.viewer.show();
    });
};

export const previewDocImage = (currentSrc: string, id: string, binding?: ProtyleBinding) => {
    fetchPost("/api/asset/getDocImageAssets", {id}, (response) => {
        if (binding) {
            const names = (response.data as string[]).filter(name => /^\/?assets\//.test(name));
            const active = () => binding.session.status === "active";
            const selected = binding.originalAssetPath(currentSrc);
            void Promise.allSettled(names.map(name => scopedUIAssetURL(name))).then(items => {
                const urls = items.flatMap(item => item.status === "fulfilled" ? [item.value] : []);
                const index = names.findIndex(name => name.replace(/^\//, "") === selected?.replace(/^\//, ""));
                const match = items[index];
                previewImages(urls, match?.status === "fulfilled" ? match.value : undefined, active);
            });
            return;
        }
        previewImages(response.data, currentSrc);
    });
};

export const previewAttrViewImages = (currentSrc: string, avID: string, viewID: string, query: string) => {
    fetchPost("/api/av/getCurrentAttrViewImages", {
        id: avID,
        viewID,
        query,
    }, (response) => {
        previewImages(response.data, currentSrc);
    });
};
