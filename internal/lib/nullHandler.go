package lib

func NullBoolean(v *bool) bool {
	if v == nil {
		return false
	}
	return *v
}
