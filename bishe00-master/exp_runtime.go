package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

func validate(resp *http.Response, body []byte, v Validation) bool {
	ok := true

	if len(v.Status) > 0 {
		match := false
		for _, s := range v.Status {
			if resp.StatusCode == s {
				match = true
				break
			}
		}
		ok = ok && match
	}

	if len(v.BodyContains) > 0 {
		lb := strings.ToLower(string(body))
		match := true
		for _, sub := range v.BodyContains {
			if !strings.Contains(lb, strings.ToLower(sub)) {
				match = false
				break
			}
		}
		ok = ok && match
	}

	if len(v.HeaderContains) > 0 {
		match := true
		for k, sub := range v.HeaderContains {
			vals := strings.ToLower(strings.Join(resp.Header.Values(k), ";"))
			if !strings.Contains(vals, strings.ToLower(sub)) {
				match = false
				break
			}
		}
		ok = ok && match
	}

	return ok
}

func substVars(s string, vars map[string]string) string {
	out := s
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func extractVars(resp *http.Response, body []byte, ex map[string]ExtractRule, vars map[string]string) {
	if ex == nil {
		return
	}
	lb := string(body)
	for name, rule := range ex {
		for _, rx := range rule.BodyRegex {
			re, err := regexp.Compile(rx)
			if err != nil {
				continue
			}
			m := re.FindStringSubmatch(lb)
			if len(m) >= 2 {
				vars[name] = m[1]
				break
			}
		}
		for hk, rx := range rule.HeaderRegex {
			vals := strings.Join(resp.Header.Values(hk), ";")
			re, err := regexp.Compile(rx)
			if err != nil {
				continue
			}
			m := re.FindStringSubmatch(vals)
			if len(m) >= 2 {
				vars[name] = m[1]
				break
			}
		}
	}
}

func buildUsageFromVars(vars map[string]string) string {
	if len(vars) == 0 {
		return ""
	}

	var parts []string
	for k, v := range vars {
		if !strings.HasPrefix(k, "_") {
			parts = append(parts, fmt.Sprintf("%s: %s", k, v))
		}
	}
	if len(parts) == 0 {
		return ""
	}

	return "提取到的信息:\n" + strings.Join(parts, "\n")
}
