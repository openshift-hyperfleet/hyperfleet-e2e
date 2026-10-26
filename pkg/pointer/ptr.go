package pointer

// ToPtr returns a pointer copy of value.
func ToPtr[T any](x T) *T {
	return &x
}

// FromPtr returns the pointer value or the zero value for the type.
func FromPtr[T any](x *T) T {
	if x == nil {
		var zero T
		return zero
	}

	return *x
}
