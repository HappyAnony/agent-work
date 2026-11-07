// Package readhttp is the first context-scoped server adapter. It is not yet
// mounted on the legacy server; production authentication/routing belongs to S05.
package readhttp

import (
	"net/http"

	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func New(manager *workspace.WorkspaceManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/read" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		for name, values := range q {
			if len(values) != 1 || (name != "storeId" && name != "pageId" && name != "boxId" && name != "docId" && name != "blockId") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		result, err := wikiops.Read(r.Context(), manager, wikiops.ReadRequest{StoreID: q.Get("storeId"), PageID: q.Get("pageId"), BoxID: q.Get("boxId"), DocID: q.Get("docId"), BlockID: q.Get("blockId")})
		if err != nil {
			writeResult(w, nil, err)
			return
		}
		writeResult(w, result, nil)
	})
}
