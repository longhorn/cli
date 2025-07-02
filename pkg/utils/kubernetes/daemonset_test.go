package kubernetes

import (
	"fmt"
	"reflect"
	"testing"
)

func TestParseNodeSelector(t *testing.T) {
	for _, test := range []struct {
		input       string
		want        map[string]string
		expectedErr error
	}{
		{
			input:       "",
			want:        nil,
			expectedErr: nil,
		},
		{
			input:       "     ",
			want:        nil,
			expectedErr: nil,
		},
		{
			input:       ",",
			want:        nil,
			expectedErr: fmt.Errorf("invalid key-value pair: \"\" (expected format key=value)"),
		},
		{
			input:       "key=value,",
			want:        nil,
			expectedErr: fmt.Errorf("invalid key-value pair: \"\" (expected format key=value)"),
		},
		{
			input:       "key1=value1,key2=value2=",
			want:        nil,
			expectedErr: fmt.Errorf("invalid key-value pair: \"key2=value2=\" (expected format key=value)"),
		},
		{
			input:       "key1=value1,key2=",
			want:        nil,
			expectedErr: fmt.Errorf("key and value must be non-empty in pair: \"key2=\""),
		},
		{
			input:       "key1=value1",
			want:        map[string]string{"key1": "value1"},
			expectedErr: nil,
		},
		{
			input:       "key1=value1,key2=value2,key3=value3",
			want:        map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
			expectedErr: nil,
		},
		{
			input:       " key1 =value1, key2= value2,   key3=value3   ",
			want:        map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
			expectedErr: nil,
		},
	} {
		got, err := ParseNodeSelector(test.input)
		if test.expectedErr != nil {
			if err == nil {
				t.Errorf("expected error but got nil")
			}
			if err.Error() != test.expectedErr.Error() {
				t.Errorf("expected error: %v, but got: %v", test.expectedErr, err)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("expected output: %v, but got:  %v", test.expectedErr, got)
			}
		}

	}
}
