package setalabs

import (
	"fmt"
	"math"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Param is one query parameter. Nested values may be a Params value, a slice, or a map.
type Param struct {
	Name  string
	Value any
}

// Params is an ordered query. Order is preserved in the query string and in the signature.
//
// A map is also accepted, but Go maps have no order, so its keys are encoded in
// sorted order. Use Params when the order has to match another Setalabs SDK.
type Params []Param

const phpFloatPrecision = 14

func buildQuery(query any) (string, error) {
	if query == nil {
		return "", nil
	}
	switch typed := query.(type) {
	case string:
		return strings.TrimLeft(typed, "?"), nil
	case Params:
		return encodeParams(typed)
	}

	value := reflect.ValueOf(query)
	if value.Kind() == reflect.Map {
		return encodeMap(value)
	}
	return "", argument("Query must be a parameter list, a map, or a string.")
}

func encodeParams(params Params) (string, error) {
	pairs := make([][2]string, 0, len(params))
	for _, param := range params {
		name, err := parameterName(param.Name)
		if err != nil {
			return "", err
		}
		if param.Value == nil {
			continue
		}
		if !isQueryValue(param.Value) {
			return "", queryTypeError(name)
		}
		if err := appendQuery(&pairs, name, param.Value); err != nil {
			return "", err
		}
	}
	return joinPairs(pairs), nil
}

func encodeMap(value reflect.Value) (string, error) {
	entries, err := sortedMapEntries(value)
	if err != nil {
		return "", err
	}
	pairs := make([][2]string, 0, len(entries))
	for _, entry := range entries {
		item := entry.value.Interface()
		if item == nil {
			continue
		}
		if !isQueryValue(item) {
			return "", queryTypeError(entry.name)
		}
		if err := appendQuery(&pairs, entry.name, item); err != nil {
			return "", err
		}
	}
	return joinPairs(pairs), nil
}

func appendQuery(pairs *[][2]string, key string, value any) error {
	if value == nil {
		return nil
	}
	if text, ok := scalarString(value, "0"); ok {
		*pairs = append(*pairs, [2]string{key, text})
		return nil
	}
	if nested, ok := value.(Params); ok {
		for _, param := range nested {
			name, err := parameterName(param.Name)
			if err != nil {
				return err
			}
			if err := appendQuery(pairs, key+"["+name+"]", param.Value); err != nil {
				return err
			}
		}
		return nil
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Map:
		entries, err := sortedMapEntries(reflected)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := appendQuery(pairs, key+"["+entry.name+"]", entry.value.Interface()); err != nil {
				return err
			}
		}
		return nil
	case reflect.Slice, reflect.Array:
		if reflected.Kind() == reflect.Slice && reflected.Type().Elem().Kind() == reflect.Uint8 {
			return queryTypeError(key)
		}
		for i := 0; i < reflected.Len(); i++ {
			item := reflected.Index(i).Interface()
			if err := appendQuery(pairs, fmt.Sprintf("%s[%d]", key, i), item); err != nil {
				return err
			}
		}
		return nil
	default:
		return queryTypeError(key)
	}
}

func isQueryValue(value any) bool {
	if value == nil {
		return true
	}
	if _, ok := scalarString(value, "0"); ok {
		return true
	}
	if _, ok := value.(Params); ok {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Map:
		return true
	case reflect.Slice, reflect.Array:
		if reflected.Kind() == reflect.Slice && reflected.Type().Elem().Kind() == reflect.Uint8 {
			return false
		}
		return true
	default:
		return false
	}
}

type mapEntry struct {
	name  string
	value reflect.Value
}

func sortedMapEntries(value reflect.Value) ([]mapEntry, error) {
	keys := value.MapKeys()
	entries := make([]mapEntry, 0, len(keys))
	for _, key := range keys {
		name, err := parameterNameValue(key)
		if err != nil {
			return nil, err
		}
		entries = append(entries, mapEntry{name: name, value: value.MapIndex(key)})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})
	return entries, nil
}

func parameterName(name string) (string, error) {
	if name == "" {
		return "", argument("Query parameter name must not be empty.")
	}
	return name, nil
}

func parameterNameValue(key reflect.Value) (string, error) {
	switch key.Kind() {
	case reflect.String:
		return parameterName(key.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(key.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(key.Uint(), 10), nil
	default:
		return "", argument("Query parameter name must not be empty.")
	}
}

func queryTypeError(name string) error {
	return sprintfArgument("Query parameter \"%s\" must be a scalar or a list.", name)
}

func joinPairs(pairs [][2]string) string {
	if len(pairs) == 0 {
		return ""
	}
	var builder strings.Builder
	for i, pair := range pairs {
		if i > 0 {
			builder.WriteByte('&')
		}
		builder.WriteString(escapeRFC3986(pair[0]))
		builder.WriteByte('=')
		builder.WriteString(escapeRFC3986(pair[1]))
	}
	return builder.String()
}

func escapeRFC3986(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func scalarString(value any, falseBool string) (string, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return "", false
	}
	switch reflected.Kind() {
	case reflect.String:
		return reflected.String(), true
	case reflect.Bool:
		if reflected.Bool() {
			return "1", true
		}
		return falseBool, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(reflected.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(reflected.Uint(), 10), true
	case reflect.Float32, reflect.Float64:
		return formatPHPFloat(reflected.Float()), true
	default:
		return "", false
	}
}

func formatPHPFloat(value float64) string {
	if math.IsNaN(value) {
		return "NAN"
	}
	if math.IsInf(value, 1) {
		return "INF"
	}
	if math.IsInf(value, -1) {
		return "-INF"
	}

	negative := math.Signbit(value)
	if value == 0 {
		if negative {
			return "-0"
		}
		return "0"
	}

	digits, decpt := significantDigits(math.Abs(value), phpFloatPrecision)
	return gcvt(negative, digits, decpt, phpFloatPrecision)
}

func significantDigits(value float64, precision int) (string, int) {
	text := strconv.FormatFloat(value, 'e', precision-1, 64)
	mantissa, exponentText, ok := strings.Cut(text, "e")
	if !ok {
		return "0", 1
	}
	exponent, err := strconv.Atoi(exponentText)
	if err != nil {
		return "0", 1
	}
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimRight(whole+fraction, "0")
	if digits == "" {
		digits = "0"
	}
	return digits, exponent + 1
}

func gcvt(negative bool, digits string, decpt, ndigit int) string {
	var builder strings.Builder
	if negative {
		builder.WriteByte('-')
	}

	if (decpt >= 0 && decpt > ndigit) || decpt < -3 {
		exponent := decpt - 1
		sign := byte('+')
		absExponent := exponent
		if exponent < 0 {
			sign = '-'
			absExponent = -exponent
		}
		builder.WriteByte(digits[0])
		builder.WriteByte('.')
		if len(digits) > 1 {
			builder.WriteString(digits[1:])
		} else {
			builder.WriteByte('0')
		}
		builder.WriteByte('E')
		builder.WriteByte(sign)
		builder.WriteString(strconv.Itoa(absExponent))
		return builder.String()
	}

	if decpt <= 0 {
		builder.WriteString("0.")
		for i := 0; i < -decpt; i++ {
			builder.WriteByte('0')
		}
		builder.WriteString(digits)
		return builder.String()
	}

	if len(digits) <= decpt {
		builder.WriteString(digits)
		for i := 0; i < decpt-len(digits); i++ {
			builder.WriteByte('0')
		}
		return builder.String()
	}

	builder.WriteString(digits[:decpt])
	builder.WriteByte('.')
	builder.WriteString(digits[decpt:])
	return builder.String()
}
