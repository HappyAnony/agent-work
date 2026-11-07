export class Tab {
    public id = "";
    public title = "";
    public model: any;
    public parent: any;
    public headElement = document.createElement("li");
    public panelElement = document.createElement("div");

    constructor(options: any = {}) {
        Object.assign(this, options);
    }

    public addModel(model: any) {
        this.model = model;
    }
}

