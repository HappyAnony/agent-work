package wikistore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Encoding/json normally accepts duplicate keys. Reject them at every level,
// including ext, before binding typed metadata. Unknown top-level keys are
// rejected; ext remains opaque JSON so caller-owned fields survive unchanged.
func decodeMetadata(data []byte, target any, required []string) error {
	tokens := json.NewDecoder(bytes.NewReader(data))
	tokens.UseNumber()
	if err := checkJSONValue(tokens, 0); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return fmt.Errorf("metadata must be an object")
	}
	for _, key := range required {
		value, ok := object[key]
		if !ok {
			return fmt.Errorf("missing field %s", key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) && key != "parentId" && key != "deletedAt" && key != "entryDocument" {
			return fmt.Errorf("null field %s", key)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func checkJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("JSON nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, nested := token.(json.Delim)
	if !nested {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("invalid or duplicate JSON key")
			}
			seen[name] = true
			if err := checkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := checkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected closing delimiter")
	}
	_, err = decoder.Token()
	return err
}
