package settingpack

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"
)

func TestProviderCodecSelectedKeyAndMalformedInputs(t *testing.T) {
	ctx := context.Background()
	key := []byte("test-providerkey")
	raw := []byte(`{"endpoint":"example.invalid","password":"synthetic"}`)
	encrypted, e := EncodeProvider(ctx, raw, key)
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := DecodeProvider(ctx, encrypted, key)
	if e != nil || !bytes.Equal(decoded, raw) {
		t.Fatal(e)
	}
	for size := 0; size < 40; size++ {
		bad := []byte(hex.EncodeToString(make([]byte, size)))
		if _, e = DecodeProvider(ctx, bad, key); e == nil {
			t.Fatal("malformed cipher accepted", size)
		}
	}
	if _, e = EncodeProvider(ctx, raw, []byte("unbound")); e == nil {
		t.Fatal("invalid key accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = DecodeProvider(canceled, encrypted, key); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
