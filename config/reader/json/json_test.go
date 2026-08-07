package json

import (
	"testing"

	"github.com/fino-io/finokit/config/source"
)

func TestReader(t *testing.T) {
	data := []byte(`{"foo": "bar", "baz": {"bar": "cat"}}`)

	testData := []struct {
		path  []string
		value string
	}{
		{
			[]string{"foo"},
			"bar",
		},
		{
			[]string{"baz", "bar"},
			"cat",
		},
	}

	r := NewReader()

	c, err := r.Merge(&source.ChangeSet{Data: data}, &source.ChangeSet{})
	if err != nil {
		t.Fatal(err)
	}

	values, err := r.Values(c)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range testData {
		cc, err := values.Get(test.path...)
		if err != nil {
			t.Fatal(err)
		}
		if v := cc.String(""); v != test.value {
			t.Fatalf("Expected %s got %s for path %v", test.value, v, test.path)
		}
	}
}
