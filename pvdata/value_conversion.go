package pvdata

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func parseTag(tag string) (name string, tags map[string]string) {
	if len(tag) > 0 {
		tags = make(map[string]string)
		pairs := strings.Split(tag, ",")
		name = pairs[0]
		pairs = pairs[1:]
		for _, pair := range pairs {
			if parts := strings.SplitN(pair, "=", 2); len(parts) == 2 {
				tags[parts[0]] = parts[1]
			} else {
				tags[pair] = ""
			}
		}
	}
	return
}

type option func(v reflect.Value) PVField

func alwaysOption(val int64) option {
	return func(v reflect.Value) PVField {
		if v.Kind() == reflect.Ptr && v.Elem().CanSet() {
			switch v.Elem().Kind() {
			case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				v.Elem().SetInt(int64(val))
			case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				v.Elem().SetUint(uint64(val))
			}
		}
		return nil
	}
}
func boundOption(bound int64) option {
	return func(v reflect.Value) PVField {
		if v.CanInterface() {
			if i, ok := v.Interface().(*string); ok {
				return &PVBoundedString{
					(*PVString)(i),
					PVSize(bound),
				}
			}
		}
		return nil
	}
}
func nameOption(name string) option {
	return func(v reflect.Value) PVField {
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
			return PVStructure{name, v.Elem()}
		}
		return nil
	}
}
func shortOption() option {
	return func(v reflect.Value) PVField {
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() == reflect.Slice {
			return PVArray{alwaysShort: true, v: v}
		}
		return nil
	}
}

func tagsToOptions(tags map[string]string) []option {
	var options []option
	if val, ok := tags["name"]; ok {
		options = append(options, nameOption(val))
	}
	if val, ok := tags["always"]; ok {
		if val, err := strconv.ParseInt(val, 0, 64); err == nil {
			options = append(options, alwaysOption(val))
		}
	}
	if val, ok := tags["bound"]; ok {
		if val, err := strconv.ParseInt(val, 0, 64); err == nil {
			options = append(options, boundOption(val))
		}
	}
	if _, ok := tags["short"]; ok {
		options = append(options, shortOption())
	}
	return options
}

func valueToPVField(v reflect.Value, options ...option) PVField {
	for _, o := range options {
		if pvf := o(v); pvf != nil {
			return pvf
		}
	}
	if v.CanInterface() {
		i := v.Interface()
		if i, ok := i.(PVField); ok {
			return i
		}
		if v.Kind() != reflect.Ptr && v.CanAddr() {
			i = v.Addr().Interface()
		}
		switch i := i.(type) {
		case *PVField:
			return *i
		case *bool:
			return (*PVBoolean)(i)
		case *int8:
			return (*PVByte)(i)
		case *uint8:
			return (*PVUByte)(i)
		case *int16:
			return (*PVShort)(i)
		case *uint16:
			return (*PVUShort)(i)
		case *int32:
			return (*PVInt)(i)
		case *uint32:
			return (*PVUInt)(i)
		case *int64:
			return (*PVLong)(i)
		case *uint64:
			return (*PVULong)(i)
		case *float32:
			return (*PVFloat)(i)
		case *float64:
			return (*PVDouble)(i)
		case *string:
			return (*PVString)(i)
		}
	}
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Slice:
		return PVArray{v: v}
	case reflect.Array:
		return PVArray{fixed: true, v: v}
	case reflect.Struct:
		return PVStructure{"", v}
	}
	return nil
}

// Encode writes vs to s.Buf.
// All items in vs must implement PVField or be a pointer to something that can be converted to a PVField.
func Encode(s *EncoderState, vs ...interface{}) error {
	for _, v := range vs {
		pvf := valueToPVField(reflect.ValueOf(v))
		if pvf == nil {
			return fmt.Errorf("can't encode %T %+v", v, v)
		}
		if err := pvf.PVEncode(s); err != nil {
			return err
		}
	}
	return nil
}
func Decode(s *DecoderState, vs ...interface{}) error {
	for _, v := range vs {
		pvf := valueToPVField(reflect.ValueOf(v))
		if pvf == nil {
			return fmt.Errorf("can't decode %#v", v)
		}
		if err := pvf.PVDecode(s); err != nil {
			return err
		}
	}
	return nil
}

type Fielder interface {
	Field() (Field, error)
}

func valueToField(v reflect.Value) (Field, error) {
	if f, ok := v.Interface().(Fielder); ok {
		return f.Field()
	}
	pvf := valueToPVField(v)
	if f, ok := pvf.(Fielder); ok {
		return f.Field()
	}
	return Field{}, fmt.Errorf("don't know how to describe %#v", v.Interface())
}
