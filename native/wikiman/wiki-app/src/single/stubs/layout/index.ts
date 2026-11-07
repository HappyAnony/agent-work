export class Layout {
    public children: any[] = [];
    public element = document.createElement("div");
    public direction = "lr";
    public resize = "";
    public type = "normal";
    public parent: any;

    constructor(options: any = {}) {
        Object.assign(this, options);
    }
}

