package ptr

func Ptr[T any](v T) *T {
	var val = v
	return &val
}
