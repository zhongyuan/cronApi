package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

// JSONMap 是 map[string]string 的 JSON 列类型。
type JSONMap map[string]string

// Value 实现 driver.Valuer。
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

// Scan 实现 sql.Scanner。
func (m *JSONMap) Scan(src any) error {
	if src == nil {
		*m = nil
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("JSONMap: unsupported type %T", src)
	}
	if len(b) == 0 {
		*m = JSONMap{}
		return nil
	}
	return json.Unmarshal(b, m)
}

// JSONSteps 是任意 JSON 数组（步骤定义 / 步骤结果）的列类型。
// 定义与结果结构不同（见 internal/service/runner），因此这里泛化为 json.RawMessage。
type JSONSteps = JSONRaw

// JSONRaw 通用 JSON 列。
type JSONRaw []byte

// Value 实现 driver.Valuer。
func (r JSONRaw) Value() (driver.Value, error) {
	if len(r) == 0 {
		return nil, nil
	}
	return string(r), nil
}

// Scan 实现 sql.Scanner。
func (r *JSONRaw) Scan(src any) error {
	if src == nil {
		*r = nil
		return nil
	}
	switch v := src.(type) {
	case []byte:
		*r = append((*r)[:0], v...)
		return nil
	case string:
		*r = append((*r)[:0], v...)
		return nil
	}
	return errors.New("JSONRaw: unsupported scan type")
}

// MarshalJSON 输出原始 JSON。
func (r JSONRaw) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return r, nil
}
