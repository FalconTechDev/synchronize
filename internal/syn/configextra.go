package syn

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
)

// Keeping settings a different version of syn wrote.
//
// Two copies of syn on one machine - the installed one and a build from
// source, say - share ~/.synchronize/config.json. Whichever saves last used
// to rewrite the file from its own struct, so anything the other version
// knew about and this one does not was silently dropped: a deploy target
// stopped being one, an app stopped being run, and nothing said so.
//
// So unknown keys are carried through. They are kept verbatim and written
// back after the fields we do know, which also means a newer version's
// settings survive a trip through an older one.

// extras holds the JSON keys a struct did not recognise.
type extras map[string]json.RawMessage

// captureExtras records whatever keys in b are not fields of v.
func captureExtras(b []byte, v any) extras {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return nil
	}
	known := jsonFieldNames(reflect.TypeOf(v))
	var kept extras
	for k, raw := range all {
		if known[k] {
			continue
		}
		if kept == nil {
			kept = extras{}
		}
		kept[k] = raw
	}
	return kept
}

// mergeExtras splices the unknown keys back into an object's JSON, after the
// fields we know, so the file keeps its usual shape.
func mergeExtras(b []byte, kept extras) ([]byte, error) {
	if len(kept) == 0 {
		return b, nil
	}
	if len(b) < 2 || b[0] != '{' || b[len(b)-1] != '}' {
		return b, nil // not an object; leave it be rather than corrupt it
	}

	names := make([]string, 0, len(kept))
	for k := range kept {
		names = append(names, k)
	}
	sort.Strings(names) // a stable file, so saving twice is not a diff

	var buf bytes.Buffer
	buf.Write(b[:len(b)-1])
	for i, k := range names {
		if i > 0 || len(b) > 2 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(kept[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// jsonFieldNames is the set of JSON keys a struct type writes.
func jsonFieldNames(t reflect.Type) map[string]bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	names := map[string]bool{}
	if t.Kind() != reflect.Struct {
		return names
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue // unexported
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := f.Name
		if tag != "" {
			if comma := bytes.IndexByte([]byte(tag), ','); comma >= 0 {
				tag = tag[:comma]
			}
			if tag != "" {
				name = tag
			}
		}
		names[name] = true
	}
	return names
}

func (c *Config) UnmarshalJSON(b []byte) error {
	type plain Config
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*c = Config(p)
	c.unknown = captureExtras(b, Config{})
	return nil
}

func (c Config) MarshalJSON() ([]byte, error) {
	type plain Config
	b, err := json.Marshal(plain(c))
	if err != nil {
		return nil, err
	}
	return mergeExtras(b, c.unknown)
}

func (r *Repo) UnmarshalJSON(b []byte) error {
	type plain Repo
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*r = Repo(p)
	r.unknown = captureExtras(b, Repo{})
	return nil
}

func (r Repo) MarshalJSON() ([]byte, error) {
	type plain Repo
	b, err := json.Marshal(plain(r))
	if err != nil {
		return nil, err
	}
	return mergeExtras(b, r.unknown)
}
