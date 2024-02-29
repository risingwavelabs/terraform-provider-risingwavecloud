package defaults

func String(v string, d string) string {
	if len(v) == 0 {
		return d
	}
	return v
}
