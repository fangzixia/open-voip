// Package datetime 统一定义服务边界的纯日期与 UTC 时间格式。
package datetime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
)

const Layout = "2006-01-02 15:04:05"
const DateLayout = "2006-01-02"

func Format(t time.Time) string { return t.UTC().Format(Layout) }

// FormatDate 保留日期自身的年月日，不因时区转换而换日。
func FormatDate(t time.Time) string { return t.Format(DateLayout) }

func Parse(value string) (time.Time, error) {
	if len(value) != len(Layout) {
		return time.Time{}, fmt.Errorf("时间必须为 YYYY-MM-DD HH:MM:SS (UTC)")
	}
	t, err := time.ParseInLocation(Layout, value, time.UTC)
	if err != nil || Format(t) != value {
		return time.Time{}, fmt.Errorf("时间必须为 YYYY-MM-DD HH:MM:SS (UTC)")
	}
	return t, nil
}

// ParseDate 严格解析 YYYY-MM-DD，得到 UTC 当日零点。
func ParseDate(value string) (time.Time, error) {
	if len(value) != len(DateLayout) {
		return time.Time{}, fmt.Errorf("日期必须为 YYYY-MM-DD")
	}
	t, err := time.ParseInLocation(DateLayout, value, time.UTC)
	if err != nil || FormatDate(t) != value {
		return time.Time{}, fmt.Errorf("日期必须为 YYYY-MM-DD")
	}
	return t, nil
}

// Date 是仅含日历日期的 JSON 字段；业务中的具体时刻仍使用 time.Time。
type Date struct{ time.Time }

func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(FormatDate(d.Time)) }

func (d *Date) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		d.Time = time.Time{}
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	parsed, err := ParseDate(value)
	if err != nil {
		return err
	}
	d.Time = parsed
	return nil
}

// DateTime 可用于需要直接调用 encoding/json 的时间字段。
type DateTime struct{ time.Time }

func (t DateTime) MarshalJSON() ([]byte, error) { return json.Marshal(Format(t.Time)) }

func (t *DateTime) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		t.Time = time.Time{}
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	parsed, err := Parse(value)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// Marshal 保留原有 JSON 标签和自定义编码行为，仅替换嵌套的 time.Time 字段。
// 纯日期字段使用 Date，或在 time.Time 字段上标注 `datetime:"date"`。
func Marshal(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var tree any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&tree); err != nil {
		return nil, err
	}
	if err := encodeTimes(reflect.ValueOf(v), &tree, false); err != nil {
		return nil, err
	}
	return json.Marshal(tree)
}

var timeType = reflect.TypeOf(time.Time{})
var dateType = reflect.TypeOf(Date{})
var dateTimeType = reflect.TypeOf(DateTime{})

func encodeTimes(value reflect.Value, node *any, dateOnly bool) error {
	if !value.IsValid() || *node == nil {
		return nil
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	if value.Type() == timeType {
		if dateOnly {
			*node = FormatDate(value.Interface().(time.Time))
		} else {
			*node = Format(value.Interface().(time.Time))
		}
		return nil
	}
	if value.Type() == dateType || value.Type() == dateTimeType {
		return nil
	}
	switch value.Kind() {
	case reflect.Struct:
		object, ok := (*node).(map[string]any)
		if !ok {
			return nil
		}
		return visitFields(value.Type(), func(field reflect.StructField, name string, embedded bool) error {
			childValue := value.FieldByIndex(field.Index)
			if embedded {
				return encodeTimes(childValue, node, false)
			}
			child, exists := object[name]
			if !exists {
				return nil
			}
			if err := encodeTimes(childValue, &child, field.Tag.Get("datetime") == "date"); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			object[name] = child
			return nil
		})
	case reflect.Slice, reflect.Array:
		items, ok := (*node).([]any)
		if !ok {
			return nil
		}
		for i := range items {
			if err := encodeTimes(value.Index(i), &items[i], dateOnly); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
	case reflect.Map:
		object, ok := (*node).(map[string]any)
		if !ok {
			return nil
		}
		for _, key := range value.MapKeys() {
			name := fmt.Sprint(key.Interface())
			child, exists := object[name]
			if !exists {
				continue
			}
			if err := encodeTimes(value.MapIndex(key), &child, dateOnly); err != nil {
				return fmt.Errorf("[%s]: %w", name, err)
			}
			object[name] = child
		}
	}
	return nil
}

// Unmarshal 识别统一格式，读取旧服务消息时也接受 RFC3339。
func Unmarshal(raw []byte, v any) error { return unmarshal(raw, v, false, true) }

// UnmarshalCurrent 严格校验时间格式，但允许未知字段。
func UnmarshalCurrent(raw []byte, v any) error { return unmarshal(raw, v, false, false) }

// UnmarshalStrict 除时间解析外，也拒绝目标结构体中不存在的字段。
func UnmarshalStrict(raw []byte, v any) error { return unmarshal(raw, v, true, false) }

func unmarshal(raw []byte, v any, strict, allowLegacy bool) error {
	var tree any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&tree); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("JSON 只能包含一个值")
	}
	target := reflect.TypeOf(v)
	if target == nil || target.Kind() != reflect.Pointer || target.Elem().Kind() == reflect.Invalid {
		return json.Unmarshal(raw, v)
	}
	if err := decodeTimes(target.Elem(), &tree, false, allowLegacy); err != nil {
		return err
	}
	normalized, err := json.Marshal(tree)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(normalized))
	if strict {
		dec.DisallowUnknownFields()
	}
	return dec.Decode(v)
}

// Decode 读取一个 JSON 值并按统一时间格式反序列化。
func Decode(r io.Reader, v any) error {
	var raw json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return err
	}
	return Unmarshal(raw, v)
}

func decodeTimes(target reflect.Type, node *any, dateOnly, allowLegacy bool) error {
	if *node == nil {
		return nil
	}
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target == timeType {
		value, ok := (*node).(string)
		if !ok {
			return nil
		}
		var parsed time.Time
		var err error
		if dateOnly {
			parsed, err = ParseDate(value)
		} else {
			parsed, err = Parse(value)
			if err != nil && allowLegacy {
				parsed, err = time.Parse(time.RFC3339Nano, value)
			}
		}
		if err != nil {
			return err
		}
		*node = parsed.UTC().Format(time.RFC3339Nano)
		return nil
	}
	if target == dateType || target == dateTimeType || target.Kind() == reflect.Interface {
		return nil
	}
	switch target.Kind() {
	case reflect.Struct:
		object, ok := (*node).(map[string]any)
		if !ok {
			return nil
		}
		return visitFields(target, func(field reflect.StructField, name string, embedded bool) error {
			if embedded {
				return decodeTimes(field.Type, node, false, allowLegacy)
			}
			key := name
			child, exists := object[key]
			if !exists {
				for candidate, value := range object {
					if strings.EqualFold(candidate, name) {
						key, child, exists = candidate, value, true
						break
					}
				}
			}
			if !exists {
				return nil
			}
			if err := decodeTimes(field.Type, &child, field.Tag.Get("datetime") == "date", allowLegacy); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			object[key] = child
			return nil
		})
	case reflect.Slice, reflect.Array:
		items, ok := (*node).([]any)
		if !ok {
			return nil
		}
		for i := range items {
			if err := decodeTimes(target.Elem(), &items[i], dateOnly, allowLegacy); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
	case reflect.Map:
		object, ok := (*node).(map[string]any)
		if !ok {
			return nil
		}
		for key, child := range object {
			if err := decodeTimes(target.Elem(), &child, dateOnly, allowLegacy); err != nil {
				return fmt.Errorf("[%s]: %w", key, err)
			}
			object[key] = child
		}
	}
	return nil
}

func visitFields(target reflect.Type, visit func(reflect.StructField, string, bool) error) error {
	for i := 0; i < target.NumField(); i++ {
		field := target.Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		embedded := field.Anonymous && name == ""
		if name == "" {
			name = field.Name
		}
		if err := visit(field, name, embedded); err != nil {
			return err
		}
	}
	return nil
}
