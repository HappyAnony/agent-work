package conf

import "testing"

func TestOriginalSQLProgressMessageRules(t *testing.T) {
	language := func(id int) string {
		return map[int]string{90: "blocks %d/%d %d %s", 93: "delete %d/%d %s", 234: "rename %d/%d %d %s", 191: "history %d/%d", 217: "asset %d/%d"}[id]
	}
	policy := SQLProgressPolicy{InsertBlocks: true, DeleteBlocks: true}
	for kind, expected := range map[string]string{"blocks": "blocks 2/3 5 detail", "delete": "delete 2/3 detail", "rename": "rename 2/3 5 detail", "history": "history 2/3", "asset": "asset 2/3"} {
		p := SQLProgress{Kind: kind, Current: 2, Total: 3, BlockCount: 5, Detail: "detail"}
		if msg, on := SQLProgressMessage(p, policy, language); !on || msg != expected {
			t.Fatal(kind, msg, on)
		}
		mobile := policy
		mobile.Mobile = true
		_, on := SQLProgressMessage(p, mobile, language)
		if on != (kind == "blocks" || kind == "delete") {
			t.Fatal("original mobile rule changed", kind, on)
		}
	}
	for _, kind := range []string{"blocks", "delete", "unknown"} {
		if _, on := SQLProgressMessage(SQLProgress{Kind: kind}, SQLProgressPolicy{}, language); on {
			t.Fatal(kind)
		}
	}
	if _, on := SQLProgressMessage(SQLProgress{Kind: "history"}, policy, nil); on {
		t.Fatal("unbound language accepted")
	}
}
