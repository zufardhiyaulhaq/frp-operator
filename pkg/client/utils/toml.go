package utils

import (
	"fmt"
	"reflect"
	"strings"
	"text/template"
)

// TemplateFuncs are the functions CLIENT_TEMPLATE uses; parse the template with them.
var TemplateFuncs = template.FuncMap{
	"quote": Quote,
}

// Quote renders v as a TOML basic string, escaping quotes, backslashes and control characters,
// so secret values and user-supplied strings can never break out of the value. Pointers are
// dereferenced (nil renders as "") so optional *string fields can be passed directly.
func Quote(v any) string {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return `""`
		}
		rv = rv.Elem()
	}
	s := ""
	if rv.IsValid() {
		s = fmt.Sprint(rv.Interface())
	}

	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
