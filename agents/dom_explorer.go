package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
	"gitlab.com/bazil/langchain-go/llm"
	"golang.org/x/net/html"
	"strconv"
	"strings"
)

func clean(htmls string) string {
	doc, _ := html.Parse(strings.NewReader(htmls))
	cleanInternal(doc)
	var buf bytes.Buffer
	_ = html.Render(&buf, doc)
	return buf.String()
}

func cleanInternal(n *html.Node) {
	// If node is a script or style element, remove it
	if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "path") {
		n.Parent.RemoveChild(n)
	}

	// If node is an element, iterate over all its attributes
	if n.Type == html.ElementNode {
		var newAttrs []html.Attribute
		for _, attr := range n.Attr {
			// If attribute is not class, keep it
			if attr.Key != "class" && attr.Key != "aria-labelledby" {
				newAttrs = append(newAttrs, attr)
			}
		}
		n.Attr = newAttrs
	}

	// Traverse the DOM tree recursively
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		cleanInternal(c)
	}
}

type DomExplorer struct {
	model             *llm.ChatCompletion
	elementFunctions  []*openai.FunctionDefine
	elementsFunctions []*openai.FunctionDefine
	selectors         map[string]string
	logger            *logrus.Entry
}

func NewDomExplorer(oai *openai.Client) *DomExplorer {
	model := llm.NewChatCompletion(oai, "gpt-3.5-turbo-16k-0613")
	elementFunctions := []*openai.FunctionDefine{{
		Name:        "getElementById",
		Description: "Returns an Element representing the element whose id property matches the specified string",
		Parameters: &openai.FunctionParams{
			Type: openai.JSONSchemaTypeObject,
			Properties: map[string]*openai.JSONSchemaDefine{
				"id": {
					Type:        openai.JSONSchemaTypeString,
					Description: "The ID of the element to locate.",
				},
			},
		},
	}, {
		Name:        "querySelector",
		Description: "Returns the first Element within the document that matches the specified selector, or group of selectors. If no matches are found, null is returned.",
		Parameters: &openai.FunctionParams{
			Type: openai.JSONSchemaTypeObject,
			Properties: map[string]*openai.JSONSchemaDefine{
				"selectors": {
					Type:        openai.JSONSchemaTypeString,
					Description: "A string containing one or more selectors to match. This string must be a valid CSS selector string.",
				},
			},
		},
	}}
	elementsFunction := []*openai.FunctionDefine{{
		Name:        "getElementsByClassName",
		Description: "Returns a list of all Elements which have all of the given class name(s).",
		Parameters: &openai.FunctionParams{
			Type: openai.JSONSchemaTypeObject,
			Properties: map[string]*openai.JSONSchemaDefine{
				"names": {
					Type:        openai.JSONSchemaTypeString,
					Description: "A string representing the class name(s) to match; multiple class names are separated by whitespace.",
				},
			},
		},
	}, {
		Name:        "getElementsByName",
		Description: "Returns a list of Elements with a given name attribute in the document.",
		Parameters: &openai.FunctionParams{
			Type: openai.JSONSchemaTypeObject,
			Properties: map[string]*openai.JSONSchemaDefine{
				"name": {
					Type:        openai.JSONSchemaTypeString,
					Description: "The value of the name attribute of the element(s) we are looking for.",
				},
			},
		},
	}, {
		Name:        "getElementsByTagName",
		Description: "Returns a list of Elements with the given tag name.",
		Parameters: &openai.FunctionParams{
			Type: openai.JSONSchemaTypeObject,
			Properties: map[string]*openai.JSONSchemaDefine{
				"name": {
					Type:        openai.JSONSchemaTypeString,
					Description: "A string representing the name of the elements. The special string * represents all elements.",
				},
			},
		},
	}, {
		Name:        "querySelectorAll",
		Description: "Returns a list of Elements that match the specified group of selectors.",
		Parameters: &openai.FunctionParams{
			Type: openai.JSONSchemaTypeObject,
			Properties: map[string]*openai.JSONSchemaDefine{
				"selectors": {
					Type:        openai.JSONSchemaTypeString,
					Description: "A string containing one or more selectors to match. This string must be a valid CSS selector string.",
				},
			},
		},
	}}
	return &DomExplorer{
		model:             model,
		elementFunctions:  elementFunctions,
		elementsFunctions: elementsFunction,
		selectors:         make(map[string]string),
		logger:            logrus.New().WithField("agent", "dom-explorer"),
	}
}

func (dom *DomExplorer) WithLogger(entry *logrus.Entry) *DomExplorer {
	dom.logger = entry
	return dom
}

func (dom *DomExplorer) GetElement(element *rod.Element, prompt string) (*rod.Element, error) {
	dom.model.WithFunctions(dom.elementFunctions)
	eval := func(js string) (interface{}, error) {
		elementObj, err := element.Evaluate(&rod.EvalOptions{JS: js, ThisObj: element.Object})
		if err != nil {
			return nil, fmt.Errorf("error evaluating javascript: %w", err)
		}
		if elementObj == nil || elementObj.ObjectID == "" {
			// No element found
			return nil, fmt.Errorf("no object")
		}
		el, err := element.Page().ElementFromObject(elementObj)
		if err != nil {
			return nil, fmt.Errorf("error getting element from object: %v", err)
		}
		return el, err
	}
	elhtml := element.MustHTML()
	el, err := dom.get(clean(elhtml), prompt, eval)
	if err != nil {
		return nil, err
	}
	if el == nil {
		return nil, nil
	} else {
		return el.(*rod.Element), nil
	}
}

func (dom *DomExplorer) GetElements(element *rod.Element, prompt string) ([]*rod.Element, error) {
	dom.model.WithFunctions(dom.elementsFunctions)
	eval := func(js string) (interface{}, error) {
		elementsObj, err := element.Evaluate(&rod.EvalOptions{JS: js, ThisObj: element.Object})
		if err != nil {
			return nil, err
		}
		if elementsObj == nil || elementsObj.ObjectID == "" {
			return nil, fmt.Errorf("no object")
		}
		// get properties of the remote object
		properties, err := proto.RuntimeGetProperties{
			ObjectID: elementsObj.ObjectID,
		}.Call(element.Page())
		if err != nil {
			return nil, err
		}
		var elements []*rod.Element
		for _, property := range properties.Result {
			if _, err := strconv.ParseUint(property.Name, 10, 64); err != nil {
				// Not an array element
				continue
			}
			el, err := element.Page().ElementFromObject(property.Value)
			if err != nil {
				return nil, err
			}
			elements = append(elements, el)
		}
		return elements, nil
	}
	elhtml := element.MustHTML()
	el, err := dom.get(clean(elhtml), prompt, eval)
	if err != nil {
		return nil, err
	}
	return el.([]*rod.Element), nil
}

func (dom *DomExplorer) get(html, prompt string, eval func(string) (interface{}, error)) (interface{}, error) {
	js, ok := dom.selectors[prompt]
	if ok {
		res, err := eval(js)
		if err == nil {
			return res, nil
		} else {
			return nil, err
		}
	}
	ntrials := 0
	for ntrials < 10 {
		queryPrompt := fmt.Sprintf(
			"Your goal is to write a function call that will select a requested element in a raw HTML.\n"+
				" - Make the selector generic.\n"+
				" - Don't use class names or ids that look generated.\n"+
				" - Don't use hardcoded values.\n"+
				" The query is '%s', the html is: ```html\n%s\n```",
			prompt, html)
		completion, err := dom.model.OAIComplete(queryPrompt)
		if err != nil {
			return nil, err
		}
		fc := completion.Choices[0].Message.FunctionCall
		if fc != nil {
			// Construct the javascript code block
			var args map[string]string
			if err := json.Unmarshal([]byte(fc.Arguments), &args); err != nil {
				return nil, fmt.Errorf("failed to unmarshal arguments: %s %s", fc.Arguments, err)
			}
			// All the function calls have only one argument
			var argsStr string
			for _, v := range args {
				argsStr = v
			}
			js := fmt.Sprintf("() => { return this.%s(\"%s\") }", fc.Name, argsStr)
			dom.logger.Debugf("trying javascript selector: %s", js)
			res, err := eval(js)
			if err != nil {
				dom.logger.Debugf("selector failed with error %v, re-trying..", err)
				ntrials += 1
				continue
			} else {
				dom.selectors[prompt] = js
				return res, nil
			}
		} else {
			return nil, fmt.Errorf("no function call")
		}
	}
	return nil, fmt.Errorf("could not satisfy success function")
}
