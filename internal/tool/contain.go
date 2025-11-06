package tool

func ContainsString(s []string, v string) bool {
	for _, vv := range s {
		if v == vv {
			return true
		}
	}
	return false
}
