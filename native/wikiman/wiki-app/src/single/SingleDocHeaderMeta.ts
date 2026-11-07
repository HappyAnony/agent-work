import {fetchPost} from "../util/fetch";

const META_REFRESH_DELAY = 240;

export class SingleDocHeaderMeta {
    private blockId: string;
    private element: HTMLElement;
    private protyle: IProtyle;
    private refreshTimer = 0;

    constructor(options: { blockId: string; protyle: IProtyle }) {
        this.blockId = options.blockId;
        this.protyle = options.protyle;
        this.element = document.createElement("div");
        this.element.className = "single-doc-meta";
        this.protyle.title?.element.appendChild(this.element);
        this.refresh();
    }

    public refreshSoon(): void {
        window.clearTimeout(this.refreshTimer);
        this.refreshTimer = window.setTimeout(() => this.refresh(), META_REFRESH_DELAY);
    }

    public handleWsMessage(data: IWebSocketData): void {
        if (!data) {
            return;
        }
        if (data.cmd === "reload" && data.data === this.blockId) {
            this.refreshSoon();
            return;
        }
        if (data.cmd === "transactions" || data.cmd === "savedoc" || data.cmd === "rename") {
            this.refreshSoon();
        }
    }

    public destroy(): void {
        window.clearTimeout(this.refreshTimer);
        this.element.remove();
    }

    private refresh(): void {
        fetchPost("/api/block/getDocInfo", {
            id: this.blockId,
        }, (response) => {
            if (response.code !== 0 || !response.data?.ial) {
                return;
            }
            const ial = response.data.ial;
            this.protyle.background?.render(ial, this.blockId);
            this.renderUpdatedTime(ial.updated);
        });
    }

    private renderUpdatedTime(updated?: string): void {
        if (!updated) {
            this.element.textContent = "";
            return;
        }
        this.element.textContent = `${window.siyuan.languages.modifiedAt || "Modified at"} ${formatWikiTime(updated)}`;
    }
}

function formatWikiTime(value: string): string {
    if (!/^\d{14}$/.test(value)) {
        return value;
    }
    return `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)} ${value.slice(8, 10)}:${value.slice(10, 12)}:${value.slice(12, 14)}`;
}
