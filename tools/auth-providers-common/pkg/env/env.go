package env

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

func LoadEnvForStruct[T any](s *T) error {
	v := reflect.ValueOf(s).Elem() // Dereference pointer to get struct value
	t := v.Type()

	if t.Kind() != reflect.Struct {
		return fmt.Errorf("expected a struct, got %v", t.Kind())
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		varNames := field.Tag.Get("env")
		if varNames == "" {
			continue
		}

		var value string
		for varName := range strings.SplitSeq(varNames, ",") {
			if value = os.Getenv(varName); value != "" {
				break
			}
		}
		if value == "" {
			defaultValue := field.Tag.Get("default")
			if defaultValue != "" {
				value = defaultValue
			} else {
				if field.Tag.Get("optional") == "true" {
					continue
				}
				return fmt.Errorf("missing required environment variable(s) %s", varNames)
			}
		}

		fieldValue := v.Field(i)
		if !fieldValue.CanSet() {
			return fmt.Errorf("cannot set value for field %s", field.Name)
		}

		// We can add more types as needed
		switch field.Type.Kind() {
		case reflect.String:
			fieldValue.SetString(value)
		case reflect.Slice:
			if field.Type.Elem().Kind() == reflect.String {
				fieldValue.Set(reflect.ValueOf(strings.Split(value, ",")))
			} else {
				return fmt.Errorf("unsupported slice element type %v for field %s", field.Type.Elem().Kind(), field.Name)
			}
		case reflect.Pointer:
			if field.Type.Elem().Kind() == reflect.String {
				fieldValue.Set(reflect.ValueOf(&value))
			} else {
				return fmt.Errorf("unsupported pointer element type %v for field %s", field.Type.Elem().Kind(), field.Name)
			}
		default:
			return fmt.Errorf("unsupported type %v for field %s", field.Type.Kind(), field.Name)
		}
	}

	return nil
}
