package utils

import (
	"reflect"
	"testing"
)

type jsonCoerceValue struct {
	Name string `json:"name"`
}

func TestJSONCoerce(t *testing.T) {
	tests := []struct {
		name string
		in   any
	}{
		{name: "map", in: map[string]any{"name": "obot"}},
		{name: "string", in: `{"name":"obot"}`},
		{name: "string pointer", in: new(`{"name":"obot"}`)},
		{name: "bytes", in: []byte(`{"name":"obot"}`)},
		{name: "same type", in: jsonCoerceValue{Name: "obot"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got jsonCoerceValue
			if err := JSONCoerce(tt.in, &got); err != nil {
				t.Fatal(err)
			}
			if want := (jsonCoerceValue{Name: "obot"}); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}

func TestJSONCoerceString(t *testing.T) {
	var got string
	if err := JSONCoerce(map[string]any{"name": "obot"}, &got); err != nil {
		t.Fatal(err)
	}
	if got != `{"name":"obot"}` {
		t.Fatalf("got %q, want JSON object", got)
	}

	if err := JSONCoerce("plain text", &got); err != nil {
		t.Fatal(err)
	}
	if got != "plain text" {
		t.Fatalf("got %q, want plain text", got)
	}
}

func TestJSONCoerceInvalidJSON(t *testing.T) {
	var got jsonCoerceValue
	if err := JSONCoerce("not JSON", &got); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}
