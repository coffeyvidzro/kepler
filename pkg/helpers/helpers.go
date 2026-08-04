// Package helpers provides small generic collection helpers that preserve input
// ordering and avoid application-specific dependencies.
package helpers

// FirstNonZero returns the first non-zero value, or the type's zero value when
// every value is zero.
func FirstNonZero[T comparable](values ...T) T {
	var zero T
	for _, value := range values {
		if value != zero {
			return value
		}
	}
	return zero
}

// Unique removes duplicate values while preserving their first-seen order.
func Unique[T comparable](values []T) []T {
	if values == nil {
		return nil
	}
	result := make([]T, 0, len(values))
	seen := make(map[T]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// Map transforms each value while preserving its position.
func Map[T, R any](values []T, transform func(T) R) []R {
	if values == nil {
		return nil
	}
	result := make([]R, len(values))
	for index, value := range values {
		result[index] = transform(value)
	}
	return result
}

// Filter returns values for which keep returns true, preserving input order.
func Filter[T any](values []T, keep func(T) bool) []T {
	if values == nil {
		return nil
	}
	result := make([]T, 0, len(values))
	for _, value := range values {
		if keep(value) {
			result = append(result, value)
		}
	}
	return result
}

// CopyMap returns a shallow copy of source. A nil map remains nil.
func CopyMap[K comparable, V any](source map[K]V) map[K]V {
	if source == nil {
		return nil
	}
	result := make(map[K]V, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
