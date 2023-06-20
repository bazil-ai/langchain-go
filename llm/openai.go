package llm

import (
	goContext "context"
	"fmt"
	"github.com/sashabaranov/go-openai"
)

type ChatCompletion struct {
	oai       *openai.Client
	modelID   string
	functions []*openai.FunctionDefine
}

func NewChatCompletion(oai *openai.Client, models ...string) *ChatCompletion {
	var modelID string
	if len(models) == 0 {
		modelID = "gpt-4"
	} else {
		modelID = models[0]
	}
	return &ChatCompletion{oai: oai, modelID: modelID}
}

func (c *ChatCompletion) WithFunctions(functions []*openai.FunctionDefine) {
	c.functions = functions
}

func (c *ChatCompletion) OAIComplete(prompt string) (*openai.ChatCompletionResponse, error) {
	res, err := c.oai.CreateChatCompletion(goContext.Background(), openai.ChatCompletionRequest{
		Model: c.modelID,
		Messages: []openai.ChatCompletionMessage{{
			Role:    "user",
			Content: prompt,
		}},
		Functions: c.functions,
	})
	if err != nil {
		return nil, fmt.Errorf("error on completion: %w", err)
	}
	return &res, nil
}

func (c *ChatCompletion) Complete(prompt string) (string, error) {
	res, err := c.oai.CreateChatCompletion(goContext.Background(), openai.ChatCompletionRequest{
		Model: c.modelID,
		Messages: []openai.ChatCompletionMessage{{
			Role:    "user",
			Content: prompt,
		}},
		Functions: c.functions,
	})
	if err != nil {
		return "", fmt.Errorf("error on chat completion: %w", err)
	}
	response := res.Choices[0].Message.Content
	return response, nil
}

type Completion struct {
	oai   *openai.Client
	model openai.Model
}

func NewCompletion(oai *openai.Client, model string) *Completion {
	return &Completion{oai: oai, model: openai.Model{ID: model}}
}

func (c *Completion) Complete(prompt string) (string, error) {
	res, err := c.oai.CreateCompletion(goContext.Background(), openai.CompletionRequest{
		Model:  c.model.ID,
		Prompt: prompt,
	})
	if err != nil {
		return "", fmt.Errorf("error on completion: %w", err)
	}
	response := res.Choices[0].Text
	return response, nil
}
