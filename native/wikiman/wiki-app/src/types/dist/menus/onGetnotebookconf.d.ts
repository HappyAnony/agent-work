declare interface INotebookConf {
    name: string;
    box: string;
    conf: {
        refCreateSavePath: string;
        docCreateSavePath: string;
        dailyNoteSavePath: string;
        refCreateSaveBox: string;
        docCreateSaveBox: string;
        dailyNoteTemplatePath: string;
    };
}
export declare const genNotebookOption: (id: string, notebookId?: string) => string;
export declare const onGetnotebookconf: (data: INotebookConf) => void;
export {};
