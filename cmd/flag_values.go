package cmd

type multiString []string

func (m *multiString) String() string { return "" }
func (m *multiString) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func stringsToAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
