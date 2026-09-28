package api

import "strings"

// bbqlEscaper escapes the characters that are special inside a BBQL quoted
// string: the backslash and the double quote.
var bbqlEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// QuoteBBQL returns s as a double-quoted BBQL string literal, escaping any
// backslashes and double quotes so user input cannot terminate the literal
// and inject further query clauses.
func QuoteBBQL(s string) string {
	return `"` + bbqlEscaper.Replace(s) + `"`
}
