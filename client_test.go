package proto

import (
	"reflect"
	"testing"
)

func TestClient(t *testing.T) {
	c := NewClient("127.0.0.1:10000")
	t.Log(reflect.TypeOf(c))
}
