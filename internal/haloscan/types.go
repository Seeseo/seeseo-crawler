package haloscan

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Number unmarshals a JSON value that may be a number, a string, or the
// literal "NA" that Haloscan returns when a value is missing. Use Float64()
// to read it back as a normal numeric value.
type Number struct {
	Value float64
	Valid bool
}

func (n *Number) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "" || s == "null" {
		return nil
	}
	if s == `"NA"` || s == "\"\"" {
		return nil
	}
	// JSON-quoted string: try to parse the unquoted content as a number.
	if s[0] == '"' {
		var raw string
		if err := json.Unmarshal(b, &raw); err != nil {
			return nil
		}
		raw = strings.TrimSpace(raw)
		if raw == "" || raw == "NA" {
			return nil
		}
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil
		}
		n.Value, n.Valid = f, true
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	n.Value, n.Valid = f, true
	return nil
}

func (n Number) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

func (n Number) Float64() float64 { return n.Value }
func (n Number) Int64() int64     { return int64(n.Value) }

// Flag unmarshals a Haloscan boolean flag that may be true, false or "NA".
// "NA" is interpreted as unknown (Valid == false).
type Flag struct {
	Value bool
	Valid bool
}

func (f *Flag) UnmarshalJSON(b []byte) error {
	s := string(b)
	switch s {
	case "true":
		f.Value, f.Valid = true, true
	case "false":
		f.Value, f.Valid = false, true
	case `"NA"`, `"true"`:
		// "NA" → unknown. "true" string → unlikely but treat as unknown to be safe.
	}
	return nil
}

func (f Flag) MarshalJSON() ([]byte, error) {
	if !f.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(f.Value)
}

// True returns the boolean value, but only when the flag is Valid AND set.
// Use this to avoid the bug where Python's bool("NA") == True caused false
// positives in the audit pipeline (cf. feedback_haloscan_brand_strict.md).
func (f Flag) True() bool { return f.Valid && f.Value }
