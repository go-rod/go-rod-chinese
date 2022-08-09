package proto

import (
	"regexp"
	"strings"
)

var regAsterisk = regexp.MustCompile(`([^\\])\*`)
var regBackSlash = regexp.MustCompile(`([^\\])\?`)

// PatternToReg FetchRequestPattern.URLPattern to regular expression
// PatternToReg 将 FetchRequestPattern.URLPattern 转为正则表达式
func PatternToReg(pattern string) string {
	if pattern == "" {
		return ""
	}

	pattern = " " + pattern
	pattern = regAsterisk.ReplaceAllString(pattern, "$1.*")
	pattern = regBackSlash.ReplaceAllString(pattern, "$1.")

	return `\A` + strings.TrimSpace(pattern) + `\z`
}
