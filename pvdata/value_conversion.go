package pvdata

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func valueToPVField(v reflect.Value, tag string) PVField {
	var tags map[string]int
	if len(tag) > 0 {
		tags = make(map[string]int)
		for _, pair := range strings.Split(tag, ",") {
			if parts := strings.SplitN(pair, "=", 2); len(parts) == 2 {
				if val, err := strconv.Atoi(parts[1]); err == nil {
					tags[parts[0]] = val
				}
			} else {
				tags[pair] = 1
			}
		}
	}
	if v.CanInterface() {
		i := v.Interface()
		if i, ok := i.(PVField); ok {
			return i
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
			if bound := tags["bound"]; bound > 0 {
				return &PVBoundedString{
					(*PVString)(i),
					PVSize(bound),
				}
			}
			return (*PVString)(i)
		}
	}
	if v.Kind() == reflect.Ptr {
		switch v.Elem().Kind() {
		case reflect.Slice:
			return PVArray{false, v.Elem()}
		case reflect.Array:
			return PVArray{true, v.Elem()}
		case reflect.Struct:
			return PVStructure{v.Elem()}
		}
	}
	return nil
}

// Encode writes vs to s.Buf.
// All items in vs must implement PVField or be a pointer to something that can be converted to a PVField.
func Encode(s *EncoderState, vs ...interface{}) error {
	for _, v := range vs {
		pvf := valueToPVField(reflect.ValueOf(v), "")
		if pvf == nil {
			return fmt.Errorf("can't encode %#v", v)
		}
		if err := pvf.PVEncode(s); err != nil {
			return err
		}
	}
	return nil
}
func Decode(s *DecoderState, vs ...interface{}) error {
	for _, v := range vs {
		if err := valueToPVField(reflect.ValueOf(v), "").PVDecode(s); err != nil {
			return err
		}
	}
	return nil
}

type Fielder interface {
	Field() Field
}

func valueToField(v reflect.Value) (Field, error) {
	if f, ok := v.Interface().(Fielder); ok {
		return f.Field(), nil
	}
	pvf := valueToPVField(v, "")
	if f, ok := pvf.(Fielder); ok {
		return f.Field(), nil
	}
	return Field{}, fmt.Errorf("don't know how to describe %#v", v.Interface())
}
