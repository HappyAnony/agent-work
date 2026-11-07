package wikistore

import "strings"

func DecodeStore(raw []byte) (Store, error) {
	var result Store
	if err := decodeMetadata(raw, &result, storeFields); err != nil {
		return result, err
	}
	if result.SchemaVersion != 1 || result.LayoutVersion != 1 || result.DocumentFormat != "docj" || !storeIDPattern.MatchString(result.StoreID) || strings.TrimSpace(result.Name) == "" {
		return result, scanFailure("invalid_store_identity", "store.json", "storeId")
	}
	return result, nil
}
