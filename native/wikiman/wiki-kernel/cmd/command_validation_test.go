package cmd

import (
	"github.com/olahol/melody"
	"testing"
)

func TestNewCommandRejectsUnboundIdentityAndBadMode(t *testing.T) {
	if NewCommand("ping", 1, nil, nil) != nil {
		t.Fatal("nil session admitted")
	}
	s := &melody.Session{}
	if NewCommand("ping", 1, nil, s) != nil {
		t.Fatal("unbound session admitted")
	}
	s.Set("app", "a")
	s.Set("id", "s")
	for _, mode := range []interface{}{"bad", 1.5, float64(3)} {
		if NewCommand("ping", 1, map[string]interface{}{"pushMode": mode}, s) != nil {
			t.Fatalf("bad mode admitted: %v", mode)
		}
	}
	command := NewCommand("ping", 1, nil, s)
	if command == nil {
		t.Fatal("valid command rejected")
	}
}
