package cyclist

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	testLe = []struct {
		e string
		t string
		j string
	}{
		{e: "", t: "", j: `{"event":"","timestamp":null}`},
		{
			e: "goo",
			t: "2009-11-10T23:00:00Z",
			j: `{"event":"goo","timestamp":"2009-11-10T23:00:00Z"}`,
		},
		{
			e: "goose",
			t: "19diggety2",
			j: `{"event":"goose","timestamp":null}`,
		},
	}
)

func TestLifecycleEvent_MarshalJSON(t *testing.T) {
	for _, tc := range testLe {
		le := newLifecycleEvent(tc.e, tc.t)
		buf := &bytes.Buffer{}
		err := json.NewEncoder(buf).Encode(le)
		assert.Nil(t, err)
		assert.JSONEq(t, tc.j, buf.String())
	}
}
