package markdownparser

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

var attributePattern = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_-]*)="([^"]*)"`)

func parseAttributes(input string) (map[string]string, error) {
	attributes := make(map[string]string)
	remainder := attributePattern.ReplaceAllStringFunc(input, func(value string) string {
		match := attributePattern.FindStringSubmatch(value)
		attributes[match[1]] = match[2]
		return ""
	})
	if strings.TrimSpace(remainder) != "" {
		return nil, fmt.Errorf("invalid directive attributes: %s", strings.TrimSpace(remainder))
	}
	return attributes, nil
}

func matchGlob(pattern, file string) bool {
	if !strings.Contains(pattern, "**") {
		matched, _ := path.Match(pattern, file)
		return matched
	}
	var expression strings.Builder
	expression.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					expression.WriteString(`(?:.*/)?`)
				} else {
					expression.WriteString(`.*`)
				}
			} else {
				expression.WriteString(`[^/]*`)
			}
		case '?':
			expression.WriteString(`[^/]`)
		case '.', '+', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
			expression.WriteByte('\\')
			expression.WriteByte(pattern[i])
		default:
			expression.WriteByte(pattern[i])
		}
	}
	expression.WriteByte('$')
	return regexp.MustCompile(expression.String()).MatchString(file)
}
