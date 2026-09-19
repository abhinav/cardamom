package boardhistory

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// writeEventV1 streams the fixed V1 event shape without allocating an encoded
// copy of large text fields. V1 objects have ASCII field names in lexical order,
// explicit nulls, and no omitted fields. Times are already normalized to UTC by
// the payload conversion. Only the closed DTO vocabulary reaches this writer;
// it is not a general replacement for encoding/json.
func writeEventV1(out io.Writer, event eventWriteV1) error {
	writer := &eventJSONWriter{out: out}
	return writer.writeValue(reflect.ValueOf(event))
}

type eventJSONWriter struct {
	out io.Writer
}

func (w *eventJSONWriter) writeValue(value reflect.Value) error {
	if !value.IsValid() {
		return errors.New("canonical JSON value is required")
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return w.writeText("null")
		}
		return w.writeValue(value.Elem())
	}
	if value.Type() == reflect.TypeFor[time.Time]() {
		return json.MarshalWrite(w.out, value.Interface())
	}
	if value.Kind() == reflect.String {
		return w.writeString(value.String())
	}
	if value.Kind() != reflect.Struct {
		return json.MarshalWrite(w.out, value.Interface(), json.Deterministic(true))
	}

	return w.writeObject(value)
}

func (w *eventJSONWriter) writeObject(value reflect.Value) error {
	members, err := eventObjectMembers(value)
	if err != nil {
		return err
	}
	if err := w.writeText("{"); err != nil {
		return err
	}
	for index, member := range members {
		if index > 0 {
			if err := w.writeText(","); err != nil {
				return err
			}
		}
		if err := w.writeString(member.name); err != nil {
			return fmt.Errorf("encode object member name %q: %w", member.name, err)
		}
		if err := w.writeText(":"); err != nil {
			return err
		}
		if err := w.writeValue(member.value); err != nil {
			return fmt.Errorf("encode object member %q: %w", member.name, err)
		}
	}
	return w.writeText("}")
}

// writeString escapes only JSON syntax and control bytes. Valid UTF-8,
// including HTML characters and Unicode line separators, remains unchanged.
// start marks the unescaped span that can be written without copying it.
func (w *eventJSONWriter) writeString(value string) error {
	if err := w.writeText(`"`); err != nil {
		return err
	}
	start := 0
	for index := 0; index < len(value); {
		octet := value[index]
		if octet >= utf8.RuneSelf {
			_, size := utf8.DecodeRuneInString(value[index:])
			if size == 1 {
				return fmt.Errorf("string contains invalid UTF-8 at byte %d", index)
			}
			index += size
			continue
		}
		var escape string
		switch octet {
		case '"':
			escape = `\"`
		case '\\':
			escape = `\\`
		case '\b':
			escape = `\b`
		case '\f':
			escape = `\f`
		case '\n':
			escape = `\n`
		case '\r':
			escape = `\r`
		case '\t':
			escape = `\t`
		default:
			if octet < ' ' {
				const hexadecimal = "0123456789abcdef"
				escape = `\u00` + string([]byte{
					hexadecimal[octet>>4], hexadecimal[octet&0x0f],
				})
			}
		}
		if escape == "" {
			index++
			continue
		}
		if err := w.writeText(value[start:index]); err != nil {
			return err
		}
		if err := w.writeText(escape); err != nil {
			return err
		}
		index++
		start = index
	}
	if err := w.writeText(value[start:]); err != nil {
		return err
	}
	return w.writeText(`"`)
}

func (w *eventJSONWriter) writeText(value string) error {
	written, err := io.WriteString(w.out, value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

// eventObjectMember retains a reference to a DTO field; text remains unencoded
// until writeString can pass its unescaped spans directly to the part writer.
type eventObjectMember struct {
	name  string
	value reflect.Value
}

func eventObjectMembers(value reflect.Value) ([]eventObjectMember, error) {
	members := make([]eventObjectMember, 0, value.NumField())
	valueType := value.Type()
	for index := range value.NumField() {
		field := valueType.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name, options, hasOptions := strings.Cut(field.Tag.Get("json"), ",")
		if hasOptions && options != "" {
			return nil, fmt.Errorf("object member %q uses unsupported JSON options", field.Name)
		}
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		members = append(members, eventObjectMember{name: name, value: value.Field(index)})
	}
	slices.SortFunc(members, func(left, right eventObjectMember) int {
		return strings.Compare(left.name, right.name)
	})
	for index := 1; index < len(members); index++ {
		if members[index-1].name == members[index].name {
			return nil, fmt.Errorf("object contains duplicate member %q", members[index].name)
		}
	}
	return members, nil
}
