export class Model {
    public element = document.createElement("div");
    public parent: any;
    public app: any;
    public ws: WebSocket | undefined;

    constructor(options: any = {}) {
        Object.assign(this, options);
    }
}

