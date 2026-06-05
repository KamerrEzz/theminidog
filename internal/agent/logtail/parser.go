package logtail

import (
	"regexp"
	"strings"
)

// Package-level compiled regexps — zero allocation per ParseLevel call.
// rePrefixed uses a non-capturing group for the optional timestamp prefix so
// the T and Z timestamp characters do not collide with the level-word capture
// under case-insensitive mode.
var (
	reJSONLevel = regexp.MustCompile(`(?i)"(?:level|severity)"\s*:\s*"(\w+)"`)
	reBracketed = regexp.MustCompile(`(?i)\[(\w+)\]`)
	rePrefixed  = regexp.MustCompile(`(?i)^(?:[\d][\d\-T:.Z\s]*)?\s*(\w+)\s*:`)
	reFirstWord = regexp.MustCompile(`(?i)^\s*\S+\s+(\w+)`)
)

var levelNorm = map[string]string{
	"error": "error", "err": "error", "critical": "error", "fatal": "error", "panic": "error",
	"warn": "warn", "warning": "warn",
	"info": "info", "information": "info",
	"debug": "debug", "dbg": "debug", "trace": "debug",
}

// ParseLevel extracts a normalized log level from a log line.
// Returns "info" when no recognizable level is found.
func ParseLevel(line string) string {
	for _, re := range []*regexp.Regexp{reJSONLevel, reBracketed, rePrefixed, reFirstWord} {
		if m := re.FindStringSubmatch(line); len(m) > 1 {
			if level, ok := levelNorm[strings.ToLower(m[1])]; ok {
				return level
			}
		}
	}
	return "info"
}
