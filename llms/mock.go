package llms

type Mock struct {
	CompleteFunc func(prompt string) (string, error)
}

func (m *Mock) Complete(prompt string) (string, error) {
	return m.CompleteFunc(prompt)
}
