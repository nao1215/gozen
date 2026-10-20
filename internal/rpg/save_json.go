package rpg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	lang "github.com/nao1215/gozen/internal/text"
)

// saveObject does not treat duplicate keys or truncated JSON as legacy-format omitted values.
func saveObject(b []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := uniqueJSON(dec); err != nil {
		return nil, fmt.Errorf(lang.T("セーブの JSON が不正。%w"), err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New(lang.T("セーブの JSON の後に余分な内容がある"))
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, errors.New(lang.T("セーブは JSON のオブジェクトであること"))
	}
	return obj, nil
}

func uniqueJSON(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return errors.New(lang.T("セーブのキーが文字列ではない"))
			}
			if seen[name] {
				return fmt.Errorf(lang.T("セーブのキー %q が重複"), name)
			}
			seen[name] = true
			if err := uniqueJSON(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSON(d); err != nil {
				return err
			}
		}
	default:
		return errors.New(lang.T("セーブの括弧が不正"))
	}
	_, err = d.Token()
	return err
}

func requiredSaveValue(obj map[string]json.RawMessage, key string, target any) error {
	b, ok := obj[key]
	if !ok || bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return fmt.Errorf(lang.T("セーブの必須項目 %s がない"), key)
	}
	if err := json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}

func decodeSave(b []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf(lang.T("セーブの構造が不正。%w"), err)
	}
	// encoding/json treats null for a number as 0 and matches key case-insensitively.
	// Both would hide corruption in saved fields, so check against the type's JSON names and the
	// original values.
	return checkSaveShape(b, reflect.TypeOf(target).Elem(), "state")
}

var rawMessageType = reflect.TypeFor[json.RawMessage]()

func checkTacticsVersions(b []byte) error {
	var actors map[string]json.RawMessage
	if err := json.Unmarshal(b, &actors); err != nil {
		return err
	}
	for id, raw := range actors {
		var actor map[string]json.RawMessage
		if err := json.Unmarshal(raw, &actor); err != nil {
			return err
		}
		if tactics, ok := actor["tactics"]; ok {
			obj, err := saveObject(tactics)
			if err != nil {
				return fmt.Errorf("actors/%s/tactics: %w", id, err)
			}
			var version int
			if err := requiredSaveValue(obj, "version", &version); err != nil {
				return fmt.Errorf("actors/%s/tactics: %w", id, err)
			}
			if version != TacticsVersion {
				return fmt.Errorf(lang.T("actors/%s/tactics の作戦形式版 %d には対応していない（対応版 %d）"), id, version, TacticsVersion)
			}
			for _, key := range []string{"mode", "rows"} {
				if _, ok := obj[key]; !ok {
					return fmt.Errorf(lang.T("セーブの必須項目 %s がない"), "actors/"+id+"/tactics/"+key)
				}
			}
		}
	}
	return nil
}

func checkSaveShape(b []byte, typ reflect.Type, path string) error {
	if typ == rawMessageType {
		return nil
	}
	if typ.Kind() == reflect.Pointer {
		return checkSaveShape(b, typ.Elem(), path)
	}
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		switch typ.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
		case reflect.Map, reflect.Slice, reflect.Interface:
			return nil
		default:
			return fmt.Errorf(lang.T("セーブの項目 %s に null は使えない"), path)
		}
	}
	switch typ.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
	case reflect.Struct:
		fields := map[string]reflect.Type{}
		for i := range typ.NumField() {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			if key == "-" {
				continue
			}
			if key == "" {
				key = f.Name
			}
			fields[key] = f.Type
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(b, &obj); err != nil {
			return err
		}
		for key, v := range obj {
			t, ok := fields[key]
			if !ok {
				return fmt.Errorf(lang.T("セーブに未知の項目 %s がある"), path+"/"+key)
			}
			if err := checkSaveShape(v, t, path+"/"+key); err != nil {
				return err
			}
		}
	case reflect.Map:
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(b, &obj); err != nil {
			return err
		}
		for key, v := range obj {
			if err := checkSaveShape(v, typ.Elem(), path+"/"+key); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		var vs []json.RawMessage
		if err := json.Unmarshal(b, &vs); err != nil {
			return err
		}
		for i, v := range vs {
			if err := checkSaveShape(v, typ.Elem(), fmt.Sprintf("%s/%d", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}
