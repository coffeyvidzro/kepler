// Package ptr provides generic pointer helpers.
package ptr

// To returns a pointer to value.
func To[T any](value T) *T {
	return &value
}

// Deref returns the pointed-to value, or the type's zero value for nil.
func Deref[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

// ValueOr returns the pointed-to value, or fallback for nil.
func ValueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

// Clone returns a pointer to a copy of value, preserving nil.
func Clone[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// Equal compares two pointers by nil state and pointed-to value.
func Equal[T comparable](left, right *T) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
