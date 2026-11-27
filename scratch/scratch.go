// Package scratch cuts the scratch folder from the paths a tool printed. The engine runs a tool in a scratch
// folder of its own and sends its name as scratch; the tool prints paths below it, which differ on every run and
// every system, so a plugin cuts them to the project's own paths before they become an entry or a finding, and
// the recordings hold none.
package scratch

import "strings"

// Path is p with the scratch folder cut from its front: a path below scratch is relative to it, scratch itself is
// ".", and any other path is as it was. Both separators are read, since a tool on Windows prints either.
func Path(p, scratch string) string {
	dir := trim(scratch)
	if dir == "" {
		return p
	}
	rest, ok := strings.CutPrefix(p, dir)
	switch {
	case !ok:
		return p
	case rest == "":
		return "."
	case rest[0] == '/' || rest[0] == '\\':
		if rest = strings.TrimLeft(rest, `/\`); rest == "" {
			return "."
		}
		return rest
	}
	return p
}

// Text is s with every path below the scratch folder cut as Path cuts it. The folder is found only where it is a
// whole path: not inside a longer name (s10 for s1) and not after a name character, so x/tmp/s1 is not cut. A
// folder with nothing after it becomes ".".
func Text(s, scratch string) string {
	dir := trim(scratch)
	if dir == "" {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, dir)
		if i < 0 {
			break
		}
		end := i + len(dir)
		if (i > 0 && nameChar(s[i-1])) || (end < len(s) && !pathEnd(s[end])) {
			b.WriteString(s[:end])
			s = s[end:]
			continue
		}
		b.WriteString(s[:i])
		if end < len(s) && (s[end] == '/' || s[end] == '\\') {
			end++
			if end >= len(s) || !nameChar(s[end]) {
				b.WriteByte('.')
			}
		} else {
			b.WriteByte('.')
		}
		s = s[end:]
	}
	b.WriteString(s)
	return b.String()
}

// trim is the scratch folder without the separators that end it.
func trim(scratch string) string { return strings.TrimRight(scratch, `/\`) }

// nameChar is whether c can be a part of a file's name or of a longer path before the folder, so that the folder
// is not a whole path there.
func nameChar(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '-' || c == '.' || c == '/' || c == '\\'
}

// pathEnd is whether c may stand right after the folder: a separator, or something that is not a name.
func pathEnd(c byte) bool { return c == '/' || c == '\\' || !nameChar(c) }
