package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

func toStringValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func toHeaderStringMap(v interface{}) map[string]string {
	out := map[string]string{}
	if v == nil {
		return out
	}
	switch m := v.(type) {
	case map[string]string:
		for k, vv := range m {
			out[k] = vv
		}
	case map[string]interface{}:
		for k, vv := range m {
			out[k] = toStringValue(vv)
		}
	case map[interface{}]interface{}:
		for kk, vv := range m {
			out[toStringValue(kk)] = toStringValue(vv)
		}
	}
	return out
}

func normalizeTargetBaseURL(allResults map[string]interface{}, pocScan map[string]interface{}) string {
	targetBaseURL := ""
	if s, ok := pocScan["target"].(string); ok {
		targetBaseURL = strings.TrimSpace(s)
	}
	if targetBaseURL == "" {
		if s, ok := allResults["target"].(string); ok {
			targetBaseURL = strings.TrimSpace(s)
		}
	}
	if targetBaseURL != "" && !strings.HasPrefix(strings.ToLower(targetBaseURL), "http://") && !strings.HasPrefix(strings.ToLower(targetBaseURL), "https://") {
		targetBaseURL = "http://" + targetBaseURL
	}
	return targetBaseURL
}

func baseURLFromResultURL(raw string) string {
	pu, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || pu == nil || pu.Scheme == "" || pu.Host == "" {
		return ""
	}
	return pu.Scheme + "://" + pu.Host
}

func buildExpSpecFromPOCResult(result map[string]interface{}, targetBaseURL string) (string, ExpSpec, bool) {
	reqMap, ok := result["req"].(map[string]interface{})
	if !ok || reqMap == nil {
		return "", ExpSpec{}, false
	}

	method := strings.ToUpper(strings.TrimSpace(toStringValue(reqMap["method"])))
	if method == "" {
		method = "GET"
	}
	path := strings.TrimSpace(toStringValue(reqMap["path"]))
	body := toStringValue(reqMap["body"])
	headers := toHeaderStringMap(reqMap["headers"])

	resultURL := toStringValue(result["url"])
	specBase := strings.TrimRight(targetBaseURL, "/")
	if specBase == "" {
		specBase = baseURLFromResultURL(resultURL)
	}
	if specBase == "" {
		return "", ExpSpec{}, false
	}

	if path == "" {
		if pu, err := url.Parse(resultURL); err == nil && pu != nil {
			path = pu.RequestURI()
		}
	}

	if method == "POST" && strings.Contains(strings.ToLower(path), "index.php?s=captcha") && strings.Contains(strings.ToLower(body), "server[request_method]=") {
		cmdRe := regexp.MustCompile(`server\[REQUEST_METHOD\]=[^&]*`)
		if !strings.Contains(body, "{{cmd_urlenc}}") && !strings.Contains(body, "{{cmd}}") {
			body = cmdRe.ReplaceAllString(body, "server[REQUEST_METHOD]={{cmd_urlenc}}")
		}
	}

	name := strings.TrimSpace(toStringValue(result["poc"]))
	if name == "" {
		name = "Generated EXP"
	}

	statusList := StatusList{200}
	if st, ok := result["status"].(int); ok && st > 0 {
		statusList = StatusList{st}
	}

	spec := ExpSpec{
		Name: name,
		Steps: []ExpStep{{
			Method:   method,
			Path:     path,
			Body:     body,
			Headers:  headers,
			Validate: Validation{Status: statusList},
		}},
	}

	return specBase, spec, true
}
