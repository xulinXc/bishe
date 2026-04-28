package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

func tryParseXRPOC(path string, b []byte) (XRPOC, bool) {
	var xp XRPOC
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		_ = json.Unmarshal(b, &xp)
	} else {
		_ = yaml.Unmarshal(b, &xp)
	}
	if xp.Expression != "" && len(xp.Rules) > 0 {
		return xp, true
	}

	content := string(b)
	if len(xp.Rules) == 0 && strings.Contains(content, "rules:") && strings.Contains(content, "r0:") {
		lines := strings.Split(content, "\n")
		rules := make(map[string]XRRule)
		inRules := false
		inRule := false
		var currentRuleName string
		var currentRequest XRRequest
		var currentExpr string

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			indent := len(line) - len(strings.TrimLeft(line, " "))
			if strings.HasPrefix(trimmed, "rules:") && indent <= 2 {
				inRules = true
				continue
			}
			if !inRules {
				continue
			}
			if matched, _ := regexp.MatchString(`^r\d+:\s*$`, trimmed); matched && indent <= 4 {
				if inRule && currentRuleName != "" {
					rules[currentRuleName] = XRRule{Request: currentRequest, Expression: currentExpr}
				}
				inRule = true
				currentRuleName = strings.TrimSuffix(trimmed, ":")
				currentRequest = XRRequest{}
				currentExpr = ""
				continue
			}
			if inRule {
				switch {
				case strings.HasPrefix(trimmed, "request:"):
				case strings.HasPrefix(trimmed, "method:"):
					currentRequest.Method = strings.TrimSpace(strings.TrimPrefix(trimmed, "method:"))
				case strings.HasPrefix(trimmed, "path:"):
					currentRequest.Path = strings.TrimSpace(strings.TrimPrefix(trimmed, "path:"))
				case strings.HasPrefix(trimmed, "body:"):
					currentRequest.Body = strings.TrimSpace(strings.TrimPrefix(trimmed, "body:"))
				case strings.HasPrefix(trimmed, "expression:"):
					currentExpr = strings.TrimSpace(strings.TrimPrefix(trimmed, "expression:"))
				}
			}
			if indent <= 2 && trimmed != "" && !strings.HasPrefix(trimmed, " ") {
				inRules = false
				if inRule && currentRuleName != "" {
					rules[currentRuleName] = XRRule{Request: currentRequest, Expression: currentExpr}
				}
				break
			}
		}
		if inRule && currentRuleName != "" {
			rules[currentRuleName] = XRRule{Request: currentRequest, Expression: currentExpr}
		}
		if len(rules) > 0 {
			xp.Rules = rules
		}
	}

	if len(xp.Rules) > 0 && xp.Expression == "" && strings.Contains(content, "expression:") {
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "expression:") {
				indent := len(line) - len(strings.TrimLeft(line, " "))
				if indent <= 2 {
					expr := strings.TrimSpace(strings.TrimPrefix(trimmed, "expression:"))
					if expr != "" {
						xp.Expression = expr
						return xp, true
					}
				}
			}
		}
	}

	return xp, xp.Expression != "" && len(xp.Rules) > 0
}

func isValidNucleiPOC(np NucleiPOC, content string, yamlErr error) bool {
	if !strings.Contains(content, "requests:") || !(strings.Contains(content, "matchers:") || strings.Contains(content, "detections:") || strings.Contains(content, "matchers-condition:")) {
		return false
	}
	if len(np.Requests) > 0 && len(np.Matchers) > 0 {
		return true
	}
	hasRequestMatchers := false
	hasDetections := false
	hasAnyMatchers := len(np.Matchers) > 0
	for _, req := range np.Requests {
		if len(req.Matchers) > 0 || req.MatchersCondition != "" {
			hasRequestMatchers = true
		}
		if len(req.Detections) > 0 {
			hasDetections = true
		}
		if len(req.Matchers) > 0 || len(req.Detections) > 0 {
			hasAnyMatchers = true
		}
	}
	if len(np.Requests) > 0 && (hasRequestMatchers || hasDetections) {
		return true
	}
	if len(np.Requests) > 0 && (np.ID != "" || np.Info.Name != "") {
		if hasAnyMatchers || strings.Contains(content, "matchers:") || strings.Contains(content, "matchers-condition:") || strings.Contains(content, "detections:") {
			return true
		}
	}
	if yamlErr != nil && strings.Contains(content, "requests:") && (np.ID != "" || strings.Contains(content, "id:")) {
		return true
	}
	return false
}

func parsePOCContent(path string, b []byte) (*POC, *XRPOC, *NucleiPOC) {
	if xp, ok := tryParseXRPOC(path, b); ok {
		return nil, &xp, nil
	}

	var np NucleiPOC
	var yamlErr error
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		yamlErr = json.Unmarshal(b, &np)
	} else {
		yamlErr = yaml.Unmarshal(b, &np)
	}
	content := string(b)
	if isValidNucleiPOC(np, content, yamlErr) {
		if yamlErr != nil && len(np.Requests) == 0 {
			fmt.Printf("[POC Load Error] 无法解析 Nuclei POC (Requests为空): %s\n", path)
			return nil, nil, nil
		}
		if yamlErr != nil {
			fmt.Printf("[POC Load Warning] YAML解析失败: %s, 错误: %v\n", path, yamlErr)
			fmt.Printf("[POC Load Info] 尝试使用部分解析的 Nuclei POC: %s\n", path)
		}
		return nil, nil, &np
	}

	var p POC
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		if err := json.Unmarshal(b, &p); err == nil {
			return &p, nil, nil
		}
	} else {
		if err := yaml.Unmarshal(b, &p); err == nil {
			return &p, nil, nil
		}
	}
	return nil, nil, nil
}

func loadAllPOCs(dir string) ([]POC, []XRPOC, []NucleiPOC, error) {
	var pocs []POC
	var xrps []XRPOC
	var nucs []NucleiPOC
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := strings.ToLower(info.Name())
		if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if p, xp, np := parsePOCContent(path, b); p != nil {
			pocs = append(pocs, *p)
		} else if xp != nil {
			xrps = append(xrps, *xp)
		} else if np != nil {
			nucs = append(nucs, *np)
		}
		return nil
	})
	return pocs, xrps, nucs, err
}

func loadAllPOCsFromFiles(files []string) ([]POC, []XRPOC, []NucleiPOC, error) {
	var pocs []POC
	var xrps []XRPOC
	var nucs []NucleiPOC
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if p, xp, np := parsePOCContent(path, b); p != nil {
			pocs = append(pocs, *p)
		} else if xp != nil {
			xrps = append(xrps, *xp)
		} else if np != nil {
			nucs = append(nucs, *np)
		}
	}
	return pocs, xrps, nucs, nil
}

func getBuiltinPocDir() string {
	cwd, _ := os.Getwd()
	builtinDir := filepath.Join(cwd, "shili", "poc")
	if st, err := os.Stat(builtinDir); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(builtinDir)
		return abs
	}
	return builtinDir
}
