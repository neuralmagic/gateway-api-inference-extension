/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import (
	"encoding/json"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// KVCacheChatCompletionRequest is a struct that represents the fields from an
// OpenAI API ChatCompletionRequest that are relevant for KV cache generation.
// Model is not included as it is contained in the LLMRequest struct.
//
// Multimodal requests are not supported in the current implementation.
type KVCacheChatCompletionRequest struct {
	Messages    []openai.ChatCompletionMessage `json:"messages"`
	Tools       []openai.Tool                  `json:"tools,omitempty"`
	ToolChoices []openai.ToolChoice            `json:"tool_choices,omitempty"`
}

// NewKVCacheChatCompletionRequest creates a new KVCacheChatCompletionRequest
// from a json request.
//
// The call marshals the input map to JSON and then unmarshals it into the
// KVCacheChatCompletionRequest struct.
func NewKVCacheChatCompletionRequest(input map[string]interface{}) (*KVCacheChatCompletionRequest, error) {
	var req KVCacheChatCompletionRequest

	if messagesRaw, ok := input["messages"]; ok {
		bytes, err := json.Marshal(messagesRaw)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(bytes, &req.Messages); err != nil {
			return nil, err
		}
	}

	if toolsRaw, ok := input["tools"]; ok {
		bytes, err := json.Marshal(toolsRaw)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(bytes, &req.Tools); err != nil {
			return nil, err
		}
	}

	if choicesRaw, ok := input["tool_choices"]; ok {
		bytes, err := json.Marshal(choicesRaw)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(bytes, &req.ToolChoices); err != nil {
			return nil, err
		}
	}

	return &req, nil
}

// ToString generates a string representation of the KVCacheChatCompletionRequest.
func (r *KVCacheChatCompletionRequest) ToString() string {
	var builder strings.Builder

	for _, msg := range r.Messages {
		builder.WriteString(msg.Role)
		builder.WriteString(":")
		builder.WriteString(msg.Content)
		builder.WriteString("\n")
	}

	if len(r.Tools) > 0 {
		toolsJSON, err := json.Marshal(r.Tools)
		if err == nil {
			builder.WriteString("tools:")
			builder.Write(toolsJSON)
			builder.WriteString("\n")
		}
	}

	if len(r.ToolChoices) > 0 {
		choicesJSON, err := json.Marshal(r.ToolChoices)
		if err == nil {
			builder.WriteString("tool_choices:")
			builder.Write(choicesJSON)
			builder.WriteString("\n")
		}
	}

	return builder.String()
}
