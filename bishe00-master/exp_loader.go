package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

func buildExpKeyInfo(baseURL string, spec ExpSpec) string {
	var sb strings.Builder
	name := strings.TrimSpace(spec.Name)
	if name != "" {
		sb.WriteString("EXP: ")
		sb.WriteString(name)
		sb.WriteString("\n")
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" {
		sb.WriteString("Target: ")
		sb.WriteString(baseURL)
		sb.WriteString("\n")
	}

	phSet := map[string]struct{}{}
	phRe := regexp.MustCompile(`\{\{([a-zA-Z0-9_]+)\}\}`)
	addPh := func(s string) {
		m := phRe.FindAllStringSubmatch(s, -1)
		for _, mm := range m {
			if len(mm) >= 2 && mm[1] != "" {
				phSet[mm[1]] = struct{}{}
			}
		}
	}

	if strings.TrimSpace(spec.ExploitSuggestion) != "" {
		addPh(spec.ExploitSuggestion)
	}

	if len(spec.Steps) > 0 {
		sb.WriteString("Steps:\n")
		for i, st := range spec.Steps {
			method := strings.ToUpper(strings.TrimSpace(st.Method))
			if method == "" {
				method = "GET"
			}
			path := strings.TrimSpace(st.Path)
			sb.WriteString(fmt.Sprintf("- #%d %s %s\n", i+1, method, path))
			addPh(st.Path)
			addPh(st.Body)
			for _, v := range st.Headers {
				addPh(v)
			}

			if len(st.Headers) > 0 {
				keys := make([]string, 0, len(st.Headers))
				for k := range st.Headers {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				sb.WriteString("  headers:\n")
				for _, k := range keys {
					v := strings.TrimSpace(st.Headers[k])
					if len(v) > 200 {
						v = v[:200] + "..."
					}
					sb.WriteString(fmt.Sprintf("    %s: %s\n", k, v))
				}
			}

			if strings.TrimSpace(st.Body) != "" {
				body := strings.TrimSpace(st.Body)
				if len(body) > 400 {
					body = body[:400] + "..."
				}
				sb.WriteString("  body:\n")
				sb.WriteString("    ")
				sb.WriteString(strings.ReplaceAll(body, "\n", "\n    "))
				sb.WriteString("\n")
			}

			if len(st.Validate.Status) > 0 || len(st.Validate.BodyContains) > 0 || len(st.Validate.HeaderContains) > 0 {
				sb.WriteString("  validate:\n")
				if len(st.Validate.Status) > 0 {
					sb.WriteString("    status: ")
					sb.WriteString(fmt.Sprintf("%v", []int(st.Validate.Status)))
					sb.WriteString("\n")
				}
				if len(st.Validate.BodyContains) > 0 {
					sb.WriteString("    bodyContains: ")
					sb.WriteString(strings.Join(st.Validate.BodyContains, ", "))
					sb.WriteString("\n")
				}
				if len(st.Validate.HeaderContains) > 0 {
					keys := make([]string, 0, len(st.Validate.HeaderContains))
					for k := range st.Validate.HeaderContains {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					sb.WriteString("    headerContains:\n")
					for _, k := range keys {
						sb.WriteString(fmt.Sprintf("      %s: %s\n", k, st.Validate.HeaderContains[k]))
					}
				}
			}

			if len(st.Extract) > 0 {
				varNames := make([]string, 0, len(st.Extract))
				for vn := range st.Extract {
					varNames = append(varNames, vn)
				}
				sort.Strings(varNames)
				sb.WriteString("  extract:\n")
				for _, vn := range varNames {
					rule := st.Extract[vn]
					sb.WriteString(fmt.Sprintf("    %s:\n", vn))
					if len(rule.BodyRegex) > 0 {
						sb.WriteString("      bodyRegex:\n")
						for _, rx := range rule.BodyRegex {
							sb.WriteString(fmt.Sprintf("        - %s\n", rx))
						}
					}
					if len(rule.HeaderRegex) > 0 {
						keys := make([]string, 0, len(rule.HeaderRegex))
						for hk := range rule.HeaderRegex {
							keys = append(keys, hk)
						}
						sort.Strings(keys)
						sb.WriteString("      headerRegex:\n")
						for _, hk := range keys {
							sb.WriteString(fmt.Sprintf("        %s: %s\n", hk, rule.HeaderRegex[hk]))
						}
					}
				}
			}
		}
	}

	if len(phSet) > 0 {
		var ph []string
		for k := range phSet {
			ph = append(ph, k)
		}
		sort.Strings(ph)
		sb.WriteString("Placeholders: ")
		sb.WriteString(strings.Join(ph, ", "))
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

func decodeExpFile(path string, content []byte) (ExpSpec, bool) {
	var spec ExpSpec
	low := strings.ToLower(path)
	if strings.HasSuffix(low, ".json") {
		if err := json.Unmarshal(content, &spec); err != nil {
			return ExpSpec{}, false
		}
	} else {
		if err := yaml.Unmarshal(content, &spec); err != nil {
			return ExpSpec{}, false
		}
	}
	if len(spec.Steps) == 0 {
		return ExpSpec{}, false
	}
	return spec, true
}

func loadExps(dir string) ([]ExpSpec, error) {
	var exps []ExpSpec
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
		if spec, ok := decodeExpFile(path, b); ok {
			exps = append(exps, spec)
		}
		return nil
	})
	return exps, err
}

func loadExpsFromFiles(files []string) ([]ExpSpec, error) {
	var exps []ExpSpec
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if spec, ok := decodeExpFile(path, b); ok {
			exps = append(exps, spec)
		}
	}
	return exps, nil
}
