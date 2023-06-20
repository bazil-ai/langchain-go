package types

type CompletionModel interface {
	Complete(prompt string) (string, error)
}

type EmbeddingModel interface {
	Embed(text string) ([]float32, error)
}
