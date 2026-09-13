package crawler

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/anurag/nexus/model"
)

var (
	reThrowHead    = regexp.MustCompile(`throw\s+new\s+([A-Z]\w*(?:Exception|Error|Throwable))\s*\(`)
	reLogHead      = regexp.MustCompile(`\b(?:log|logger|LOG|LOGGER)\s*\.\s*(error|warn)\s*\(`)
	reStringLit    = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
	reErrorCode    = regexp.MustCompile(`\b([A-Z][A-Z0-9]{2,}(?:_[A-Z0-9]+)+)\b`)
)

const maxThrowSites = 80

var throwStopwords = map[string]bool{
	"the": true, "for": true, "and": true, "not": true, "with": true,
	"this": true, "that": true, "cannot": true, "could": true, "unable": true,
	"failed": true, "error": true, "exception": true, "invalid": true,
	"while": true, "when": true, "from": true, "into": true, "was": true,
	"are": true,
}

func extractThrowSites(source string, imports []string, methods []model.Method) []model.ThrowSite {
	if source == "" {
		return nil
	}

	importMap := buildImportMap(imports)

	var sites []model.ThrowSite
	seen := map[string]bool{}

	for _, loc := range reThrowHead.FindAllStringIndex(source, -1) {
		if len(sites) >= maxThrowSites {
			break
		}
		match := reThrowHead.FindStringSubmatch(source[loc[0]:loc[1]])
		if len(match) < 2 {
			continue
		}
		simple := match[1]
		fqn := resolveImport(simple, importMap)
		line := countLines(source, loc[0])
		msg := argMessage(source, loc[1])
		key := "THROW|" + fqn + "|" + itoa(line)
		if seen[key] {
			continue
		}
		seen[key] = true
		sites = append(sites, model.ThrowSite{
			ExceptionFQN: fqn,
			ErrorCode:    extractErrorCode(msg),
			Tokens:       messageTokens(msg),
			Method:       methodAtLine(methods, line),
			Line:         line,
			Kind:         "THROW",
		})
	}

	for _, loc := range reLogHead.FindAllStringIndex(source, -1) {
		if len(sites) >= maxThrowSites {
			break
		}
		match := reLogHead.FindStringSubmatch(source[loc[0]:loc[1]])
		if len(match) < 2 {
			continue
		}
		level := strings.ToLower(match[1])
		kind := "LOG_ERROR"
		if level == "warn" {
			kind = "LOG_WARN"
		}
		line := countLines(source, loc[0])
		msg := argMessage(source, loc[1])
		key := kind + "||" + itoa(line)
		if seen[key] {
			continue
		}
		seen[key] = true
		code := extractErrorCode(msg)
		tokens := messageTokens(msg)
		if code == "" && len(tokens) == 0 {
			continue
		}
		sites = append(sites, model.ThrowSite{
			ErrorCode: code,
			Tokens:    tokens,
			Method:    methodAtLine(methods, line),
			Line:      line,
			Kind:      kind,
		})
	}

	return sites
}

func buildImportMap(imports []string) map[string]string {
	m := make(map[string]string, len(imports))
	for _, imp := range imports {
		imp = strings.TrimPrefix(imp, "static ")
		if dot := strings.LastIndex(imp, "."); dot >= 0 && dot+1 < len(imp) {
			simple := imp[dot+1:]
			if simple != "*" {
				m[simple] = imp
			}
		}
	}
	return m
}

func resolveImport(simple string, importMap map[string]string) string {
	if fqn, ok := importMap[simple]; ok {
		return fqn
	}
	return simple
}

func argMessage(source string, argStart int) string {
	semi := strings.Index(source[argStart:], ";")
	end := argStart + 300
	if semi >= 0 && argStart+semi < end {
		end = argStart + semi
	}
	if end > len(source) {
		end = len(source)
	}
	if end <= argStart {
		return ""
	}
	window := source[argStart:end]
	var sb strings.Builder
	for _, match := range reStringLit.FindAllStringSubmatch(window, -1) {
		if sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		s := match[1]
		s = strings.ReplaceAll(s, `\"`, `"`)
		s = strings.ReplaceAll(s, `\n`, " ")
		s = strings.ReplaceAll(s, `\t`, " ")
		s = strings.ReplaceAll(s, `\\`, `\`)
		sb.WriteString(s)
	}
	return sb.String()
}

func extractErrorCode(msg string) string {
	m := reErrorCode.FindStringSubmatch(msg)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

var rePlaceholder = regexp.MustCompile(`\{[^}]*\}|%[-0-9.]*[sdfn]|\$\{[^}]*\}`)

func messageTokens(msg string) []string {
	if msg == "" {
		return nil
	}
	cleaned := rePlaceholder.ReplaceAllString(msg, " ")
	cleaned = strings.ToLower(cleaned)

	var tokens []string
	seen := map[string]bool{}
	for _, t := range splitNonAlnum(cleaned) {
		if len(t) < 3 || throwStopwords[t] || isAllDigits(t) || seen[t] {
			continue
		}
		seen[t] = true
		tokens = append(tokens, t)
		if len(tokens) >= 12 {
			break
		}
	}
	return tokens
}

func splitNonAlnum(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func methodAtLine(methods []model.Method, line int) string {
	best := ""
	bestLine := 0
	for _, m := range methods {
		if m.Line > 0 && m.Line <= line && m.Line > bestLine {
			bestLine = m.Line
			best = m.Name
		}
	}
	return best
}

func countLines(source string, offset int) int {
	if offset <= 0 {
		return 1
	}
	limit := offset
	if limit > len(source) {
		limit = len(source)
	}
	line := 1
	for i := 0; i < limit; i++ {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
