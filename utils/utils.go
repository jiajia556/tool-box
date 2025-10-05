package utils

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func CopyStructFields(src, dst interface{}) error {
	srcVal := reflect.ValueOf(src)
	dstVal := reflect.ValueOf(dst)

	if srcVal.Kind() != reflect.Ptr || dstVal.Kind() != reflect.Ptr {
		return errors.New("src and dst must be pointers")

	}

	srcVal = srcVal.Elem()
	dstVal = dstVal.Elem()

	if srcVal.Kind() != reflect.Struct || dstVal.Kind() != reflect.Struct {
		return errors.New("src and dst must be struct types")
	}

	srcType := srcVal.Type()

	for i := 0; i < srcVal.NumField(); i++ {
		srcField := srcVal.Field(i)
		srcFieldType := srcType.Field(i)

		dstField := dstVal.FieldByName(srcFieldType.Name)
		if !dstField.IsValid() {
			continue
		}

		if dstField.Type() == srcField.Type() && dstField.CanSet() {
			dstField.Set(srcField)
		}
	}
	return nil
}

func MapToStruct(data map[string]any, out interface{}) error {
	v := reflect.ValueOf(out)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return errors.New("out must be a pointer to struct")

	}

	v = v.Elem()

	for key, value := range data {
		fieldName := snakeToCamel(key)
		field := v.FieldByName(fieldName)
		if !field.IsValid() || !field.CanSet() {
			continue
		}

		val := reflect.ValueOf(value)

		if converted, ok := convertValue(val, field.Type()); ok {
			field.Set(converted)
		}
	}

	return nil
}

func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + strings.ToLower(parts[i][1:])
		}
	}
	return strings.Join(parts, "")
}

func convertValue(val reflect.Value, targetType reflect.Type) (reflect.Value, bool) {
	if !val.IsValid() {
		return reflect.Value{}, false
	}

	for (val.Kind() == reflect.Interface || val.Kind() == reflect.Ptr) && !val.IsNil() {
		val = val.Elem()
		if !val.IsValid() {
			return reflect.Value{}, false
		}
	}

	if val.Type().AssignableTo(targetType) {
		return val, true
	}
	if val.Type().ConvertibleTo(targetType) {
		return val.Convert(targetType), true
	}

	switch targetType.Kind() {
	case reflect.String:
		return reflect.ValueOf(fmt.Sprint(val.Interface())), true

	case reflect.Bool:
		switch val.Kind() {
		case reflect.String:
			s := strings.TrimSpace(strings.ToLower(val.String()))
			if s == "1" || s == "true" || s == "yes" || s == "on" {
				return reflect.ValueOf(true), true
			}
			if s == "0" || s == "false" || s == "no" || s == "off" {
				return reflect.ValueOf(false), true
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return reflect.ValueOf(val.Int() != 0).Convert(targetType), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return reflect.ValueOf(val.Uint() != 0).Convert(targetType), true
		case reflect.Float32, reflect.Float64:
			return reflect.ValueOf(val.Float() != 0).Convert(targetType), true
		}
		return reflect.Value{}, false

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch val.Kind() {
		case reflect.String:
			if i, err := strconv.ParseInt(strings.TrimSpace(val.String()), 0, targetType.Bits()); err == nil {
				return reflect.ValueOf(i).Convert(targetType), true
			}
		case reflect.Float32, reflect.Float64:
			return reflect.ValueOf(int64(val.Float())).Convert(targetType), true
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return reflect.ValueOf(val.Int()).Convert(targetType), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return reflect.ValueOf(int64(val.Uint())).Convert(targetType), true
		case reflect.Bool:
			if val.Bool() {
				return reflect.ValueOf(int64(1)).Convert(targetType), true
			}
			return reflect.ValueOf(int64(0)).Convert(targetType), true
		}
		return reflect.Value{}, false

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		switch val.Kind() {
		case reflect.String:
			if u, err := strconv.ParseUint(strings.TrimSpace(val.String()), 0, targetType.Bits()); err == nil {
				return reflect.ValueOf(u).Convert(targetType), true
			}
		case reflect.Float32, reflect.Float64:
			f := val.Float()
			if f < 0 {
				f = 0
			}
			return reflect.ValueOf(uint64(f)).Convert(targetType), true
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			i := val.Int()
			if i < 0 {
				i = 0
			}
			return reflect.ValueOf(uint64(i)).Convert(targetType), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return reflect.ValueOf(val.Uint()).Convert(targetType), true
		case reflect.Bool:
			if val.Bool() {
				return reflect.ValueOf(uint64(1)).Convert(targetType), true
			}
			return reflect.ValueOf(uint64(0)).Convert(targetType), true
		}
		return reflect.Value{}, false

	case reflect.Float32, reflect.Float64:
		switch val.Kind() {
		case reflect.String:
			if f, err := strconv.ParseFloat(strings.TrimSpace(val.String()), targetType.Bits()); err == nil {
				return reflect.ValueOf(f).Convert(targetType), true
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return reflect.ValueOf(float64(val.Int())).Convert(targetType), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return reflect.ValueOf(float64(val.Uint())).Convert(targetType), true
		case reflect.Float32, reflect.Float64:
			return reflect.ValueOf(val.Float()).Convert(targetType), true
		case reflect.Bool:
			if val.Bool() {
				return reflect.ValueOf(float64(1)).Convert(targetType), true
			}
			return reflect.ValueOf(float64(0)).Convert(targetType), true
		}
		return reflect.Value{}, false
	}

	return reflect.Value{}, false
}
