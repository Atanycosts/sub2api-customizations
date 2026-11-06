package service

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// MemoryJSONUnambiguous rejects text for which GJSON's first-key selection and
// encoding/json's last-key selection could disagree. Case aliases are also
// rejected because encoding/json matches struct fields case-insensitively.
// These limits apply to memory participation only, not ordinary forwarding.
func MemoryJSONUnambiguous(data []byte) bool {
	if len(data) == 0 || len(data) > 8*1024*1024 || !utf8.Valid(data) || !gjson.ValidBytes(data) {
		return false
	}
	return memoryJSONKeysUnique(gjson.ParseBytes(data), 0)
}

func memoryJSONKeysUnique(value gjson.Result, depth int) bool {
	if depth > 64 {
		return false
	}
	if !value.IsObject() && !value.IsArray() {
		return true
	}
	var seen map[string]bool
	if value.IsObject() {
		seen = make(map[string]bool)
	}
	valid := true
	value.ForEach(func(key, child gjson.Result) bool {
		if seen != nil {
			name := memoryJSONFoldKey(key.String())
			if seen[name] {
				valid = false
				return false
			}
			seen[name] = true
		}
		valid = memoryJSONKeysUnique(child, depth+1)
		return valid
	})
	return valid
}

func memoryJSONFoldKey(key string) string {
	return strings.Map(func(r rune) rune {
		canonical := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < canonical {
				canonical = next
			}
		}
		return canonical
	}, key)
}
