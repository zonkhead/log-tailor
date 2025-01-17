package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	logger "log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func stderrln(s string) {
	fmt.Fprintln(os.Stderr, s)
}

func stderrf(fs string, a ...any) {
	fmt.Fprintf(os.Stderr, fs, a...)
}

func stderr(_struct any) {
	_yaml, _ := yaml.Marshal(_struct)
	stderrln(string(_yaml))
}

func capitalize(s string) string {
	return strings.ToUpper(string(s[0])) + s[1:]
}

func hasKeys[K comparable, V any](m map[K]V, ks ...K) bool {
	for _, k := range ks {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

func strVal(m OutputMap, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// Given a source string, a regex, and a value output string it returns a
// value. Example output string: resource-$1-$2
func regexVal(src, regex, val string) string {
	re := regexp.MustCompile(regex)
	m := re.FindStringSubmatch(src)
	// Only support 9 groups for now.
	result := val
	if len(m) > 0 {
		for i := 1; i <= min(9, len(m)-1); i++ {
			result = strings.ReplaceAll(result, `$`+strconv.Itoa(i), m[i])
		}
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Gets the path elements from a string. It allows for both pure dot syntax
// and the map key-specifier syntax:
// resource.labels.dataset_id
// labels.key(authorization.k8s.io/decision)
func pathElements(path string) []string {
	prefix := "key("
	suffix := ")"
	if !strings.Contains(path, prefix) {
		return strings.Split(path, ".")
	}
	var result []string
	re := strings.Split(path, ".")
	for i := 0; i < len(re); i++ {
		e := re[i]
		if strings.HasPrefix(e, prefix) {
			var b strings.Builder
			b.WriteString(strings.Replace(e, prefix, "", -1))
			for i++; i < len(re) && !strings.HasSuffix(re[i], suffix); i++ {
				b.WriteString("." + re[i])
			}
			if i >= len(re) {
				logger.Println("Parse error on key(): " + path)
				break
			}
			b.WriteString("." + strings.Replace(re[i], suffix, "", -1))
			result = append(result, b.String())
		} else {
			result = append(result, e)
		}
	}
	return result
}
