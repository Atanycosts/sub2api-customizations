package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryJSONRejectsAmbiguousAndExcessivelyNestedObjects(t *testing.T) {
	for _, input := range []string{
		`{"input":"first","input":"last"}`,
		`{"status":"completed","\u0073tatus":"incomplete"}`,
		`{"status":"completed","Status":"incomplete"}`,
		`{"status":"completed","ſtatus":"incomplete"}`,
		`{"status":"completed","\u017ftatus":"incomplete"}`,
		`{"outer":[{"text":"first","text":"last"}]}`,
		strings.Repeat("[", 66) + "1" + strings.Repeat("]", 66),
		`{"text":"` + string([]byte{0xff}) + `"}`,
	} {
		require.False(t, MemoryJSONUnambiguous([]byte(input)), "input=%q", input)
	}
	for _, input := range []string{
		`{"input":"a literal \\\"input\\\": is harmless","other":{"input":"separate object"}}`,
		`{"items":[{"text":"first"},{"text":"second"}]}`,
		`{"input":"中文🧠"}`,
	} {
		require.True(t, MemoryJSONUnambiguous([]byte(input)), "input=%q", input)
	}
}
