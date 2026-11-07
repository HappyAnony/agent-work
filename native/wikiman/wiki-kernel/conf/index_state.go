package conf

// SetIndexPending is the original SQL-index change/flush callback business.
// This is runtime state, never a classified persisted preference or source save.
func (data *AppData) SetIndexPending(pending bool) {
	data.DataIndexState = 0
	if pending {
		data.DataIndexState = 1
	}
}
