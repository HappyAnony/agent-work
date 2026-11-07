package server

import (
	"math"
	"wiki-kernel/util"
)

// Validate the complete envelope before NewCommand consumes typed fields.
func parseWebSocketCommand(request map[string]interface{}) (name string, id float64, param map[string]interface{}, valid bool) {
	name, nameOK := request["cmd"].(string)
	id, idOK := request["reqId"].(float64)
	param, paramOK := request["param"].(map[string]interface{})
	if !nameOK || name == "" || !idOK || math.IsNaN(id) || math.IsInf(id, 0) || !paramOK || param == nil {
		return name, id, param, false
	}
	if mode, exists := param["pushMode"]; exists && mode != nil {
		value, ok := mode.(float64)
		if !ok || !util.ValidPushMode(value) {
			return name, id, param, false
		}
	}
	return name, id, param, true
}
