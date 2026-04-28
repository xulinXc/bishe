package main

type POC struct {
	Name         string            `json:"name"`
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	Body         string            `json:"body"`
	Match        string            `json:"match"`
	Headers      map[string]string `json:"headers"`
	MatchHeaders map[string]string `json:"matchHeaders"`
	MatchBodyAny []string          `json:"matchBodyAny"`
	MatchBodyAll []string          `json:"matchBodyAll"`
	Retry        int               `json:"retry"`
	RetryDelayMs int               `json:"retryDelayMs"`
}

type XRRequest struct {
	Method  string            `yaml:"method" json:"method"`
	Path    string            `yaml:"path" json:"path"`
	Body    string            `yaml:"body" json:"body"`
	Headers map[string]string `yaml:"headers" json:"headers"`
}

type XRRule struct {
	Request    XRRequest `yaml:"request" json:"request"`
	Expression string    `yaml:"expression" json:"expression"`
}

type XRInfo struct {
	Name      string   `yaml:"name" json:"name"`
	Author    string   `yaml:"author" json:"author"`
	Severity  string   `yaml:"severity" json:"severity"`
	Reference []string `yaml:"reference" json:"reference"`
}

type XRPOC struct {
	ID         string            `yaml:"id" json:"id"`
	Info       XRInfo            `yaml:"info" json:"info"`
	Rules      map[string]XRRule `yaml:"rules" json:"rules"`
	Expression string            `yaml:"expression" json:"expression"`
}

type NucleiMatcher struct {
	Type      string   `yaml:"type" json:"type"`
	Part      string   `yaml:"part" json:"part"`
	Words     []string `yaml:"words" json:"words"`
	Regex     []string `yaml:"regex" json:"regex"`
	Condition string   `yaml:"condition" json:"condition"`
	Status    []int    `yaml:"status" json:"status"`
	Dsl       []string `yaml:"dsl" json:"dsl"`
}

type NucleiHeaders struct {
	MapHeaders   map[string]string
	ArrayHeaders []interface{}
}

func (h *NucleiHeaders) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var mapHeaders map[string]string
	if err := unmarshal(&mapHeaders); err == nil {
		h.MapHeaders = mapHeaders
		return nil
	}
	var arrayHeaders []interface{}
	if err := unmarshal(&arrayHeaders); err == nil {
		h.ArrayHeaders = arrayHeaders
		return nil
	}
	return nil
}

type NucleiRequest struct {
	Raw               []string        `yaml:"raw" json:"raw"`
	Method            string          `yaml:"method" json:"method"`
	Path              []string        `yaml:"path" json:"path"`
	URL               string          `yaml:"url" json:"url"`
	Redirect          bool            `yaml:"redirect" json:"redirect"`
	Headers           interface{}     `yaml:"headers" json:"headers"`
	Body              string          `yaml:"body" json:"body"`
	FollowRedirects   bool            `yaml:"follow_redirects" json:"follow_redirects"`
	Detections        []string        `yaml:"detections" json:"detections"`
	MatchersCondition string          `yaml:"matchers-condition" json:"matchers-condition"`
	Matchers          []NucleiMatcher `yaml:"matchers" json:"matchers"`
}

type NucleiInfo struct {
	Name        string   `yaml:"name" json:"name"`
	Author      string   `yaml:"author" json:"author"`
	Severity    string   `yaml:"severity" json:"severity"`
	Risk        string   `yaml:"risk" json:"risk"`
	Reference   []string `yaml:"reference" json:"reference"`
	Description string   `yaml:"description" json:"description"`
	Tags        []string `yaml:"tags" json:"tags"`
}

type NucleiPOC struct {
	ID                string          `yaml:"id" json:"id"`
	Info              NucleiInfo      `yaml:"info" json:"info"`
	Params            []interface{}   `yaml:"params" json:"params"`
	Variables         []interface{}   `yaml:"variables" json:"variables"`
	Requests          []NucleiRequest `yaml:"requests" json:"requests"`
	MatchersCondition string          `yaml:"matchers-condition" json:"matchers-condition"`
	Matchers          []NucleiMatcher `yaml:"matchers" json:"matchers"`
	MaxRedirects      int             `yaml:"max-redirects" json:"max-redirects"`
	Reference         []interface{}   `yaml:"reference" json:"reference"`
}

type PocScanReq struct {
	BaseURL     string   `json:"baseUrl"`
	PocDir      string   `json:"pocDir"`
	PocPaths    []string `json:"pocPaths"`
	TimeoutMs   int      `json:"timeoutMs"`
	Concurrency int      `json:"concurrency"`
}
