package setup

// Constructors for fields that live in a part of the draft: ptr finds that part.

// Text is a one-line field.
func TextField[D any](key, flag, label, help string, ptr func(D) *string) Field[D] {
	return Field[D]{
		Key: key, Flag: flag, Label: label, Help: help, Kind: Text,
		Get: func(d D) any { return *ptr(d) },
		Set: func(d D, v any) error { *ptr(d) = v.(string); return nil },
	}
}

// ListField is a free list of values.
func ListField[D any](key, flag, label, help string, ptr func(D) *[]string) Field[D] {
	return Field[D]{
		Key: key, Flag: flag, Label: label, Help: help, Kind: List,
		Get: func(d D) any { return *ptr(d) },
		Set: func(d D, v any) error { *ptr(d) = v.([]string); return nil },
	}
}

// MultiField is any of the options.
func MultiField[D any](key, flag, label, help string, options func(D) []Option, ptr func(D) *[]string) Field[D] {
	f := ListField(key, flag, label, help, ptr)
	f.Kind, f.OptionsOf = Multi, options
	return f
}

// Fixed are options of a fixed list of values.
func Fixed[D any](values ...string) func(D) []Option {
	return func(D) []Option {
		out := make([]Option, len(values))
		for i, v := range values {
			out[i] = Option{Value: v}
		}
		return out
	}
}
