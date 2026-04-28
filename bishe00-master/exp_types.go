package main

import (
	"encoding/json"
	"strconv"
)

type Validation struct {
	Status         StatusList        `json:"status"`
	BodyContains   []string          `json:"bodyContains"`
	HeaderContains map[string]string `json:"headerContains"`
}

type StatusList []int

func parseStatusList(data interface{}) []int {
	var result []int

	switch v := data.(type) {
	case []int:
		return v
	case []interface{}:
		result = make([]int, 0, len(v))
		for _, item := range v {
			switch val := item.(type) {
			case int:
				result = append(result, val)
			case float64:
				result = append(result, int(val))
			case string:
				if val != "" && val != "suspect" {
					if i, err := strconv.Atoi(val); err == nil {
						result = append(result, i)
					}
				}
			}
		}
		return result
	case string:
		if v == "" || v == "suspect" {
			return []int{}
		}
		if i, err := strconv.Atoi(v); err == nil {
			return []int{i}
		}
	case int:
		return []int{v}
	case float64:
		return []int{int(v)}
	}
	return []int{}
}

func (s *StatusList) UnmarshalJSON(data []byte) error {
	var intList []int
	if err := json.Unmarshal(data, &intList); err == nil {
		*s = intList
		return nil
	}

	var strList []string
	if err := json.Unmarshal(data, &strList); err == nil {
		*s = parseStatusList(strList)
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = parseStatusList(str)
		return nil
	}

	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*s = []int{i}
		return nil
	}

	*s = []int{}
	return nil
}

func (s *StatusList) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var intList []int
	if err := unmarshal(&intList); err == nil {
		*s = intList
		return nil
	}

	var strList []string
	if err := unmarshal(&strList); err == nil {
		*s = parseStatusList(strList)
		return nil
	}

	var str string
	if err := unmarshal(&str); err == nil {
		*s = parseStatusList(str)
		return nil
	}

	var i int
	if err := unmarshal(&i); err == nil {
		*s = []int{i}
		return nil
	}

	*s = []int{}
	return nil
}

type ExtractRule struct {
	BodyRegex   []string          `json:"bodyRegex"`
	HeaderRegex map[string]string `json:"headerRegex"`
}

type ExpStep struct {
	Method       string                 `json:"method"`
	Path         string                 `json:"path"`
	Body         string                 `json:"body"`
	Headers      map[string]string      `json:"headers"`
	Validate     Validation             `json:"validate"`
	Extract      map[string]ExtractRule `json:"extract"`
	Retry        int                    `json:"retry"`
	RetryDelayMs int                    `json:"retryDelayMs"`
	SleepMs      int                    `json:"sleepMs"`
}

type ExpSpec struct {
	Name              string    `json:"name"`
	Steps             []ExpStep `json:"steps"`
	ExploitSuggestion string    `json:"exploitSuggestion"`
}

type ExpExecReq struct {
	BaseURL     string    `json:"baseUrl"`
	ExpDir      string    `json:"expDir"`
	ExpPaths    []string  `json:"expPaths"`
	InlineExps  []ExpSpec `json:"inlineExps"`
	Concurrency int       `json:"concurrency"`
	TimeoutMs   int       `json:"timeoutMs"`
}
