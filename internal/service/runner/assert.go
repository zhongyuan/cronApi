package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// CheckContext 断言所需上下文。
type CheckContext struct {
	Status   int
	Header   http.Header
	Body     []byte
	BodyJSON any // 解析后的 JSON，解析失败为 nil
}

// 把断言跑一遍，返回失败描述列表（空 = 全部通过）。
func RunAsserts(asserts []Assert, cc CheckContext) []string {
	var failures []string
	for _, a := range asserts {
		if msg := checkOne(a, cc); msg != "" {
			failures = append(failures, msg)
		}
	}
	return failures
}

func checkOne(a Assert, cc CheckContext) string {
	op := strings.ToLower(strings.TrimSpace(a.Op))
	src := a.Source
	if src == "" {
		src = "body"
	}
	label := a.Message
	if label == "" {
		label = fmt.Sprintf("%s %s %s", src, op, a.Value)
	}

	var actual string
	var exists bool
	switch {
	case src == "status":
		actual, exists = strconv.Itoa(cc.Status), true
	case strings.HasPrefix(src, "header:"):
		name := strings.TrimSpace(strings.TrimPrefix(src, "header:"))
		v := cc.Header.Get(name)
		actual, exists = v, v != ""
	case strings.HasPrefix(src, "json:"):
		path := strings.TrimPrefix(src, "json:")
		if cc.BodyJSON != nil {
			actual, exists = GetPathString(cc.BodyJSON, path)
		}
	default: // body
		actual = string(bytes.TrimSpace(cc.Body))
		exists = len(actual) > 0
	}

	ok := false
	switch op {
	case "eq":
		ok = exists && actual == a.Value
	case "ne":
		ok = actual != a.Value
	case "contains":
		ok = exists && strings.Contains(actual, a.Value)
	case "not_contains":
		ok = !strings.Contains(actual, a.Value)
	case "exists":
		ok = exists
	case "not_exists":
		ok = !exists
	case "len_eq":
		ok = exists && strconv.Itoa(len([]rune(actual))) == a.Value
	case "gt", "gte", "lt", "lte":
		x, err1 := strconv.ParseFloat(actual, 64)
		y, err2 := strconv.ParseFloat(a.Value, 64)
		if err1 != nil || err2 != nil {
			return label + " [数值转换失败]"
		}
		switch op {
		case "gt":
			ok = x > y
		case "gte":
			ok = x >= y
		case "lt":
			ok = x < y
		case "lte":
			ok = x <= y
		}
	case "status_in":
		for _, s := range strings.Split(a.Value, ",") {
			if strconv.Itoa(cc.Status) == strings.TrimSpace(s) {
				ok = true
				break
			}
		}
	case "json_eq":
		// 语义比较：期望值为 JSON 串，忽略字段顺序/空白
		var exp any
		if err := json.Unmarshal([]byte(a.Value), &exp); err != nil {
			return label + " [期望值 JSON 非法]"
		}
		if !jsonEqual(cc.BodyJSON, exp) {
			return fmt.Sprintf("%s [json 不一致]", label)
		}
		ok = true
	default:
		return "未知断言操作符: " + a.Op
	}
	if !ok {
		return fmt.Sprintf("%s [实际值: %s]", label, truncate(actual, 200))
	}
	return ""
}

func jsonEqual(a, b any) bool {
	ba, err1 := json.Marshal(normalizeNumbers(a))
	bb, err2 := json.Marshal(normalizeNumbers(b))
	return err1 == nil && err2 == nil && bytes.Equal(ba, bb)
}

// normalizeNumbers 把 float64 整数转 int64，避免 1 与 1.0 序列化不一致。
func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = normalizeNumbers(vv)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = normalizeNumbers(vv)
		}
		return out
	default:
		return v
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
