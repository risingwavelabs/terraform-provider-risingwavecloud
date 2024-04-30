package utils

func IfElse[T any](cond bool, v T, d T) T {
	if cond {
		return v
	}
	return d
}
