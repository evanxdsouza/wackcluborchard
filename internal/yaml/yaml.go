// Package yaml parses the subset of YAML that compose files and
// kubeconfigs use in practice: block mappings and sequences, plain and
// quoted scalars, flow sequences and mappings, literal and folded blocks,
// and comments. Anchors, tags and multi-document streams are not handled.
package yaml

import (
	"fmt"
	"strconv"
	"strings"
)

type line struct {
	indent int
	text   string // without indentation, comments stripped
	num    int
}

type parser struct {
	lines []line
	pos   int
	raw   []string
}

// Parse returns map[string]any, []any, or a scalar.
func Parse(src string) (any, error) {
	p := &parser{raw: strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")}
	for i, r := range p.raw {
		if strings.TrimSpace(r) == "---" && len(p.lines) == 0 {
			continue
		}
		t := stripComment(r)
		if strings.TrimSpace(t) == "" {
			p.lines = append(p.lines, line{indent: -1, num: i})
			continue
		}
		if strings.Contains(r[:len(r)-len(strings.TrimLeft(r, " \t"))], "\t") {
			return nil, fmt.Errorf("line %d: tabs are not allowed for indentation", i+1)
		}
		ind := len(t) - len(strings.TrimLeft(t, " "))
		p.lines = append(p.lines, line{indent: ind, text: strings.TrimRight(t[ind:], " "), num: i})
	}
	p.skipBlank()
	if p.pos >= len(p.lines) {
		return map[string]any{}, nil
	}
	v, err := p.parseBlock(p.lines[p.pos].indent)
	if err != nil {
		return nil, err
	}
	p.skipBlank()
	if p.pos < len(p.lines) {
		l := p.lines[p.pos]
		return nil, fmt.Errorf("line %d: unexpected content %q", l.num+1, l.text)
	}
	return v, nil
}

func stripComment(s string) string {
	inS, inD := false, false
	for i, r := range s {
		switch r {
		case '\'':
			if !inD {
				inS = !inS
			}
		case '"':
			if !inS {
				inD = !inD
			}
		case '#':
			if !inS && !inD && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
				return s[:i]
			}
		}
	}
	return s
}

func (p *parser) skipBlank() {
	for p.pos < len(p.lines) && p.lines[p.pos].indent < 0 {
		p.pos++
	}
}

func (p *parser) parseBlock(indent int) (any, error) {
	p.skipBlank()
	if p.pos >= len(p.lines) {
		return nil, nil
	}
	l := p.lines[p.pos]
	if l.text == "-" || strings.HasPrefix(l.text, "- ") {
		return p.parseSeq(indent)
	}
	return p.parseMap(indent)
}

func (p *parser) parseSeq(indent int) (any, error) {
	var out []any
	for {
		p.skipBlank()
		if p.pos >= len(p.lines) {
			break
		}
		l := p.lines[p.pos]
		if l.indent < indent {
			break
		}
		if l.indent > indent {
			return nil, fmt.Errorf("line %d: bad indentation", l.num+1)
		}
		if !(l.text == "-" || strings.HasPrefix(l.text, "- ")) {
			break
		}
		rest := strings.TrimSpace(strings.TrimPrefix(l.text, "-"))
		if rest == "" {
			p.pos++
			p.skipBlank()
			if p.pos < len(p.lines) && p.lines[p.pos].indent > indent {
				v, err := p.parseBlock(p.lines[p.pos].indent)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			} else {
				out = append(out, nil)
			}
			continue
		}
		// "- key: value" starts an inline mapping whose further keys are
		// indented to line up with "key".
		childIndent := indent + (len(l.text) - len(strings.TrimLeft(strings.TrimPrefix(l.text, "-"), " ")))
		if _, _, ok := splitKey(rest); ok && rest[0] != '[' && rest[0] != '{' {
			p.lines[p.pos] = line{indent: childIndent, text: rest, num: l.num}
			v, err := p.parseMap(childIndent)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		p.pos++
		v, err := p.scalarOrBlock(rest, indent)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if out == nil {
		out = []any{}
	}
	return out, nil
}

func isQuoted(s string) bool {
	return strings.HasPrefix(s, `"`) || strings.HasPrefix(s, `'`)
}

func isQuotedKey(s string) bool {
	if !isQuoted(s) {
		return false
	}
	q := s[0]
	end := strings.IndexByte(s[1:], q)
	if end < 0 {
		return false
	}
	rest := strings.TrimSpace(s[end+2:])
	return strings.HasPrefix(rest, ":")
}

// splitKey splits "key: value" honoring quoted keys.
func splitKey(s string) (string, string, bool) {
	if isQuoted(s) {
		q := s[0]
		end := strings.IndexByte(s[1:], q)
		if end < 0 {
			return "", "", false
		}
		key := s[1 : end+1]
		rest := strings.TrimSpace(s[end+2:])
		if !strings.HasPrefix(rest, ":") {
			return "", "", false
		}
		return key, strings.TrimSpace(rest[1:]), true
	}
	for i := 0; i < len(s); i++ {
		if s[i] == ':' && (i == len(s)-1 || s[i+1] == ' ') {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
		}
		if s[i] == '[' || s[i] == '{' {
			return "", "", false
		}
	}
	return "", "", false
}

func (p *parser) parseMap(indent int) (any, error) {
	out := map[string]any{}
	for {
		p.skipBlank()
		if p.pos >= len(p.lines) {
			break
		}
		l := p.lines[p.pos]
		if l.indent < indent {
			break
		}
		if l.indent > indent {
			return nil, fmt.Errorf("line %d: bad indentation", l.num+1)
		}
		if strings.HasPrefix(l.text, "- ") || l.text == "-" {
			break
		}
		k, rest, ok := splitKey(l.text)
		if !ok {
			return nil, fmt.Errorf("line %d: expected \"key: value\", got %q", l.num+1, l.text)
		}
		p.pos++
		v, err := p.scalarOrBlock(rest, indent)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// scalarOrBlock parses the value after "key:" or "- ".
func (p *parser) scalarOrBlock(rest string, indent int) (any, error) {
	switch {
	case rest == "":
		p.skipBlank()
		if p.pos < len(p.lines) {
			nl := p.lines[p.pos]
			if nl.indent > indent {
				return p.parseBlock(nl.indent)
			}
			// sequences may sit at the same indent as their key
			if nl.indent == indent && (strings.HasPrefix(nl.text, "- ") || nl.text == "-") {
				return p.parseSeq(indent)
			}
		}
		return nil, nil
	case rest[0] == '|' || rest[0] == '>':
		return p.blockScalar(rest, indent), nil
	case rest[0] == '[' || rest[0] == '{':
		// flow collections may span lines
		buf := rest
		for !balanced(buf) && p.pos < len(p.lines) {
			if p.lines[p.pos].indent >= 0 {
				buf += " " + p.lines[p.pos].text
			}
			p.pos++
		}
		v, _, err := parseFlow(buf, 0)
		return v, err
	}
	// multi-line plain or quoted scalars continue on deeper lines
	buf := rest
	for p.pos < len(p.lines) && p.lines[p.pos].indent > indent {
		if _, _, isKey := splitKey(p.lines[p.pos].text); isKey && !isQuoted(rest) {
			break
		}
		buf += " " + p.lines[p.pos].text
		p.pos++
	}
	return scalar(buf), nil
}

func balanced(s string) bool {
	depth := 0
	inS, inD := false, false
	for _, r := range s {
		switch {
		case r == '\'' && !inD:
			inS = !inS
		case r == '"' && !inS:
			inD = !inD
		case inS || inD:
		case r == '[' || r == '{':
			depth++
		case r == ']' || r == '}':
			depth--
		}
	}
	return depth <= 0
}

func (p *parser) blockScalar(header string, indent int) string {
	folded := header[0] == '>'
	chomp := ""
	if strings.Contains(header, "-") {
		chomp = "strip"
	} else if strings.Contains(header, "+") {
		chomp = "keep"
	}
	var lines []string
	blockIndent := -1
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		raw := p.raw[l.num]
		if strings.TrimSpace(raw) == "" {
			lines = append(lines, "")
			p.pos++
			continue
		}
		ind := len(raw) - len(strings.TrimLeft(raw, " "))
		if ind <= indent {
			break
		}
		if blockIndent < 0 {
			blockIndent = ind
		}
		if ind < blockIndent {
			break
		}
		lines = append(lines, raw[blockIndent:])
		p.pos++
	}
	// trailing blank lines belong to chomping, not content
	trail := 0
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
		trail++
	}
	var s string
	if folded {
		var b strings.Builder
		for i, l := range lines {
			if i > 0 {
				if l == "" || lines[i-1] == "" {
					b.WriteString("\n")
				} else {
					b.WriteString(" ")
				}
			}
			b.WriteString(l)
		}
		s = b.String()
	} else {
		s = strings.Join(lines, "\n")
	}
	switch chomp {
	case "strip":
	case "keep":
		s += strings.Repeat("\n", trail+1)
	default:
		s += "\n"
	}
	return s
}

func scalar(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if s[0] == '"' {
		if v, err := strconv.Unquote(s); err == nil {
			return v
		}
		return strings.Trim(s, `"`)
	}
	if s[0] == '\'' {
		return strings.ReplaceAll(strings.Trim(s, "'"), "''", "'")
	}
	switch s {
	case "~", "null", "Null", "NULL":
		return nil
	case "true", "True", "TRUE", "yes", "on":
		return true
	case "false", "False", "FALSE", "no", "off":
		return false
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return int(i)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && strings.ContainsAny(s, ".eE") && !strings.ContainsAny(s, ":") {
		return f
	}
	return s
}

func parseFlow(s string, i int) (any, int, error) {
	skip := func() {
		for i < len(s) && (s[i] == ' ' || s[i] == '\n') {
			i++
		}
	}
	skip()
	if i >= len(s) {
		return nil, i, fmt.Errorf("unexpected end of flow collection")
	}
	switch s[i] {
	case '[':
		i++
		out := []any{}
		for {
			skip()
			if i < len(s) && s[i] == ']' {
				return out, i + 1, nil
			}
			v, ni, err := parseFlow(s, i)
			if err != nil {
				return nil, ni, err
			}
			out = append(out, v)
			i = ni
			skip()
			if i < len(s) && s[i] == ',' {
				i++
				continue
			}
			if i < len(s) && s[i] == ']' {
				return out, i + 1, nil
			}
			return nil, i, fmt.Errorf("expected , or ] in flow sequence")
		}
	case '{':
		i++
		out := map[string]any{}
		for {
			skip()
			if i < len(s) && s[i] == '}' {
				return out, i + 1, nil
			}
			kv, ni, err := flowToken(s, i, ":")
			if err != nil {
				return nil, ni, err
			}
			i = ni
			key, _ := scalar(kv).(string)
			if key == "" {
				key = fmt.Sprint(scalar(kv))
			}
			if i < len(s) && s[i] == ':' {
				i++
			}
			v, ni2, err := parseFlow(s, i)
			if err != nil {
				return nil, ni2, err
			}
			out[key] = v
			i = ni2
			skip()
			if i < len(s) && s[i] == ',' {
				i++
				continue
			}
			if i < len(s) && s[i] == '}' {
				return out, i + 1, nil
			}
			return nil, i, fmt.Errorf("expected , or } in flow mapping")
		}
	default:
		tok, ni, err := flowToken(s, i, ",]}")
		return scalar(tok), ni, err
	}
}

func flowToken(s string, i int, stops string) (string, int, error) {
	start := i
	if i < len(s) && (s[i] == '"' || s[i] == '\'') {
		q := s[i]
		i++
		for i < len(s) && s[i] != q {
			if s[i] == '\\' && q == '"' {
				i++
			}
			i++
		}
		return s[start : i+1], i + 1, nil
	}
	for i < len(s) && !strings.ContainsRune(stops, rune(s[i])) {
		i++
	}
	return strings.TrimSpace(s[start:i]), i, nil
}

// ---- typed helpers ----

func Map(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func List(v any) []any {
	l, _ := v.([]any)
	return l
}

func String(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// Strings accepts a list, or a single string split on whitespace.
func Strings(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			out = append(out, String(x))
		}
		return out
	case string:
		return strings.Fields(t)
	}
	return nil
}
