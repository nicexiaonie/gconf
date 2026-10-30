package apollo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/clbanning/mxj"
	"github.com/magiconair/properties"
	toml "github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"
)

// sourceFormat 返回 namespace 在 Apollo 中的源格式：文件型取名称后缀，其余为 properties 型。
func sourceFormat(ns string) Format {
	if f := formatFromExt(ns); f != "" {
		return Format(f)
	}
	return FormatProperties
}

// isStructured 判断格式是否可解析为结构化 map。
func isStructured(format Format) bool {
	switch format {
	case FormatJSON, FormatYAML, FormatYML, FormatXML:
		return true
	}
	return false
}

func isKnownFormat(format Format) bool {
	switch format {
	case FormatJSON, FormatYAML, FormatYML, FormatXML, FormatTOML, FormatProperties, FormatTXT:
		return true
	}
	return false
}

// render 按 namespace 配置生成落盘内容。
// 目标格式与源格式相同时原样落地（文件型写原文，properties 型写为 properties 文件），否则解析后转换。
// 返回的错误均为配置内容错误。
func render(nsCfg NamespaceConfig, snap Snapshot) ([]byte, error) {
	src := sourceFormat(nsCfg.Name)
	dst := resolveFormat(nsCfg)

	var values map[string]any
	if !isFileType(nsCfg.Name) {
		values = snap.Values
	} else {
		raw := []byte(snap.RawContent)
		if dst == src {
			if !nsCfg.SkipValidate && isStructured(src) {
				if _, err := decode(raw, src); err != nil {
					return nil, fmt.Errorf("parse %s: %w", src, err)
				}
			}
			return raw, nil
		}
		var err error
		if values, err = decode(raw, src); err != nil {
			return nil, fmt.Errorf("parse %s: %w", src, err)
		}
	}

	data, err := encode(values, dst)
	if err != nil {
		return nil, fmt.Errorf("convert %s to %s: %w", src, dst, err)
	}
	// 自检：转换结果必须能按目标格式解析回来
	if _, err := decode(data, dst); err != nil {
		return nil, fmt.Errorf("validate %s: %w", dst, err)
	}
	return data, nil
}

func encode(values map[string]any, format Format) ([]byte, error) {
	clean := canonicalize(values)
	switch format {
	case FormatJSON:
		return json.MarshalIndent(clean, "", "    ")
	case FormatYAML, FormatYML:
		return yaml.Marshal(clean)
	case FormatXML:
		return mxj.Map(clean).XmlIndent("", "    ")
	case FormatTOML:
		return encodeTOML(clean)
	case FormatProperties:
		return encodeProperties(clean)
	}
	return nil, fmt.Errorf("unsupported target format: %s", format)
}

func decode(data []byte, format Format) (map[string]any, error) {
	switch format {
	case FormatJSON:
		return decodeJSON(data)
	case FormatYAML, FormatYML:
		out := make(map[string]any)
		if err := yaml.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return out, nil
	case FormatXML:
		m, err := mxj.NewMapXml(data)
		if err != nil {
			return nil, err
		}
		return map[string]any(m), nil
	case FormatTOML:
		tree, err := toml.LoadBytes(data)
		if err != nil {
			return nil, err
		}
		return tree.ToMap(), nil
	case FormatProperties:
		loader := &properties.Loader{Encoding: properties.UTF8, DisableExpansion: true}
		p, err := loader.LoadBytes(data)
		if err != nil {
			return nil, err
		}
		out := make(map[string]any, p.Len())
		for _, k := range p.Keys() {
			out[k], _ = p.Get(k)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported format: %s", format)
}

// decodeJSON 解析 JSON，并按原文区分整数与小数，避免整数被读成 float64 而丢失精度。
func decodeJSON(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	out := make(map[string]any)
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after top-level value")
	}
	if err := normalizeNumbers(out); err != nil {
		return nil, err
	}
	return out, nil
}

// normalizeNumbers 将 json.Number 原地替换为 int64（整数）或 float64（其余）。
func normalizeNumbers(v any) error {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			n, err := numberValue(e)
			if err != nil {
				return err
			}
			x[k] = n
		}
	case []any:
		for i, e := range x {
			n, err := numberValue(e)
			if err != nil {
				return err
			}
			x[i] = n
		}
	}
	return nil
}

func numberValue(v any) (any, error) {
	n, ok := v.(json.Number)
	if !ok {
		return v, normalizeNumbers(v)
	}
	if i, err := n.Int64(); err == nil {
		return i, nil
	}
	return n.Float64()
}

func encodeTOML(values map[string]any) ([]byte, error) {
	if err := checkTOML("", values); err != nil {
		return nil, err
	}
	tree, err := toml.TreeFromMap(values)
	if err != nil {
		return nil, err
	}
	s, err := tree.ToTomlString()
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

// checkTOML 拒绝 TOML 无法表达的值：null、超出 int64 的整数、元素类型不一致的数组。
// go-toml 遇到类型不一致的数组会 panic，必须在转换前拦截。
func checkTOML(path string, v any) error {
	switch x := v.(type) {
	case nil:
		return fmt.Errorf("%s: toml does not support null", path)
	case uint64:
		if x > math.MaxInt64 {
			return fmt.Errorf("%s: integer %d exceeds toml int64 range", path, x)
		}
	case map[string]any:
		for _, k := range sortedKeys(x) {
			if err := checkTOML(joinPath(path, k), x[k]); err != nil {
				return err
			}
		}
	case []any:
		kind := ""
		for i, e := range x {
			p := fmt.Sprintf("%s[%d]", path, i)
			if err := checkTOML(p, e); err != nil {
				return err
			}
			k := tomlKind(e)
			if kind == "" {
				kind = k
			} else if k != kind {
				return fmt.Errorf("%s: toml array elements must be the same type, got %s and %s", p, kind, k)
			}
		}
	}
	return nil
}

func tomlKind(v any) string {
	switch v.(type) {
	case map[string]any:
		return "table"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "bool"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "float"
	case time.Time:
		return "datetime"
	}
	return fmt.Sprintf("%T", v)
}

// encodeProperties 将嵌套 map 展开为 a.b=c 形式；数组、null 与展开后重复的 key 无法表达，直接报错。
func encodeProperties(values map[string]any) ([]byte, error) {
	flat := make(map[string]string)
	if err := flattenProperties("", values, flat); err != nil {
		return nil, err
	}
	p := properties.NewProperties()
	p.DisableExpansion = true
	p.WriteSeparator = "="
	for _, k := range sortedKeys(flat) {
		if _, _, err := p.Set(k, flat[k]); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	if _, err := p.Write(&buf, properties.UTF8); err != nil {
		return nil, err
	}
	// properties 库写出时不转义 key 中的 '=' 等字符，读回结果可能与原值不同，逐项比对拦截
	back, err := decode(buf.Bytes(), FormatProperties)
	if err != nil {
		return nil, err
	}
	if len(back) != len(flat) {
		return nil, fmt.Errorf("some keys cannot be represented in properties")
	}
	for k, v := range flat {
		if back[k] != v {
			return nil, fmt.Errorf("%s: key or value cannot be represented in properties", k)
		}
	}
	return buf.Bytes(), nil
}

func flattenProperties(path string, v any, out map[string]string) error {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range sortedKeys(x) {
			if k == "" {
				return fmt.Errorf("%s: properties does not support empty key", path)
			}
			if err := flattenProperties(joinPath(path, k), x[k], out); err != nil {
				return err
			}
		}
		return nil
	case nil:
		return fmt.Errorf("%s: properties does not support null", path)
	case []any:
		return fmt.Errorf("%s: properties does not support array", path)
	}
	s, err := scalarString(v)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if _, ok := out[path]; ok {
		return fmt.Errorf("%s: duplicate key after flattening", path)
	}
	out[path] = s
	return nil
}

func scalarString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case bool:
		return strconv.FormatBool(x), nil
	case int:
		return strconv.Itoa(x), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case time.Time:
		return x.Format(time.RFC3339Nano), nil
	}
	return "", fmt.Errorf("unsupported value type %T", v)
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
