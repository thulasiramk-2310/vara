// Package color provides zero-dependency ANSI terminal color helpers.
//
// Color output is auto-detected from the stdout file mode. Set NO_COLOR=1
// to disable unconditionally, or VARA_COLOR=always to force it on.
package color

import (
	"os"
	"strings"
)

var enabled = detectColor()

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	if v := os.Getenv("VARA_COLOR"); v != "" {
		v = strings.ToLower(v)
		return v == "always" || v == "1" || v == "true"
	}
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

func wrap(code, s string) string {
	if !enabled {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func Green(s string) string   { return wrap("32", s) }
func Yellow(s string) string  { return wrap("33", s) }
func Red(s string) string     { return wrap("31", s) }
func Cyan(s string) string    { return wrap("36", s) }
func Bold(s string) string    { return wrap("1", s) }
func Dim(s string) string     { return wrap("2", s) }
func BoldRed(s string) string { return wrap("1;31", s) }
