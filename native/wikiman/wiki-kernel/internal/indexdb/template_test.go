//go:build fts5

package indexdb

import (
	"context"
	"database/sql"
	"testing"
)

func TestTemplateSQLFunctionsAreOwnerLocalReadOnlyAndBounded(t *testing.T) {
	ctx := context.Background()
	for _, owner := range []string{"A", "B"} {
		t.Run(owner, func(t *testing.T) {
			b, e := OpenMemory(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer b.Close()
			db, _ := b.Database(Main)
			if e = Transaction(ctx, db, func(tx *sql.Tx) error {
				return InsertBlocks(ctx, tx, []BlockRecord{{ID: "same", RootID: "root", Box: "box", Path: "/root.docj", HPath: "/" + owner, Type: "p", Content: owner}}, false, nil)
			}); e != nil {
				t.Fatal(e)
			}
			funcs := TemplateFunctions(ctx, db)
			rows, e := funcs["queryBlocks"].(func(string, ...string) ([]BlockRecord, error))(`SELECT * FROM blocks WHERE id='?'`, "same")
			if e != nil || len(rows) != 1 || rows[0].Content != owner {
				t.Fatal(rows, e)
			}
			block, e := funcs["getBlock"].(func(any) (*BlockRecord, error))(map[string]interface{}{"id": "same"})
			if e != nil || block == nil || block.HPath != "/"+owner {
				t.Fatal(block, e)
			}
			query := funcs["querySQL"].(func(string) ([]map[string]interface{}, error))
			for _, s := range []string{"DELETE FROM blocks", "SELECT 1;DELETE FROM blocks", "ATTACH DATABASE 'other' AS other", "PRAGMA database_list"} {
				if _, e = query(s); e == nil {
					t.Fatal("unsafe SQL accepted", s)
				}
			}
			result, e := query("SELECT content FROM blocks WHERE id='same'")
			if e != nil || len(result) != 1 || result[0]["content"] != owner {
				t.Fatal(result, e)
			}
			if _, e = query("WITH RECURSIVE seq(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM seq WHERE x<1025) SELECT x FROM seq"); e == nil {
				t.Fatal("explicit/unbounded rows exceeded template budget")
			}
			c, cancel := context.WithCancel(ctx)
			cancel()
			cancelled := TemplateFunctions(c, db)["querySQL"].(func(string) ([]map[string]interface{}, error))
			if _, e = cancelled("SELECT 1"); e == nil {
				t.Fatal("cancel ignored")
			}
		})
	}
}
