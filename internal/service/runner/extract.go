package runner

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// GetPath 按点号路径从 JSON 值中取值，支持数组下标：data.list.0.id
// 也兼容 JSONPath 前缀：$.data.id
func GetPath(root any, path string) (any, bool) {
	path = strings.TrimPrefix(strings.TrimSpace(path), "$")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return root, true
	}
	cur := root
	for _, seg := range strings.Split(path, ".") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// GetPathString 取路径并转成字符串（用于变量提取）。
func GetPathString(root any, path string) (string, bool) {
	v, ok := GetPath(root, path)
	if !ok {
		return "", false
	}
	return toString(v), true
}

func toString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		return x.String()
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(b)
	}
}
