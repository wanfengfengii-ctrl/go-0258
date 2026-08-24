package inspection

import (
	"encoding/json"
	"sort"
)

// mustCanonicalJSON renders v as deterministic JSON. Map keys are sorted and
// struct fields follow their declaration order, so two equal values produce
// byte-identical output. It is used only for idempotency digests, never for
// user-facing responses, and therefore panics on values that cannot encode.
func mustCanonicalJSON(v any) string {
	b, err := marshalCanonical(v)
	if err != nil {
		panic("inspection: cannot canonicalize: " + err.Error())
	}
	return string(b)
}

func marshalCanonical(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []byte
		out = append(out, '{')
		for i, k := range keys {
			if i > 0 {
				out = append(out, ',')
			}
			kb, _ := json.Marshal(k)
			vb, err := marshalCanonical(t[k])
			if err != nil {
				return nil, err
			}
			out = append(out, kb...)
			out = append(out, ':')
			out = append(out, vb...)
		}
		out = append(out, '}')
		return out, nil
	case []any:
		var out []byte
		out = append(out, '[')
		for i, item := range t {
			if i > 0 {
				out = append(out, ',')
			}
			ib, err := marshalCanonical(item)
			if err != nil {
				return nil, err
			}
			out = append(out, ib...)
		}
		out = append(out, ']')
		return out, nil
	default:
		return json.Marshal(v)
	}
}
