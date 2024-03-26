package defaults

func String(v string, d string) string {
	if len(v) == 0 {
		return d
	}
	return v
}

func UnwrapOr[T any](v *T, u T) T {
	if v == nil {
		return u
	}
	return *v
}
