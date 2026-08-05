package moolre

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type ResponseStatus int

func (status *ResponseStatus) UnmarshalJSON(data []byte) error {
	if status == nil {
		return fmt.Errorf("%w: response status target is nil", ErrInvalidResponse)
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*status = 0
		return nil
	}

	var value int
	if data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return fmt.Errorf("decode Moolre response status: %w", err)
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil {
			return fmt.Errorf("%w: response status %q is not numeric", ErrInvalidResponse, text)
		}
		value = parsed
	} else if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode Moolre response status: %w", err)
	}

	*status = ResponseStatus(value)
	return nil
}

func (status ResponseStatus) Successful() bool { return status == 1 }

type Message json.RawMessage

func (message *Message) UnmarshalJSON(data []byte) error {
	if message == nil {
		return fmt.Errorf("%w: response message target is nil", ErrInvalidResponse)
	}
	*message = append((*message)[:0], data...)
	return nil
}

func (message Message) String() string {
	data := bytes.TrimSpace(message)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		return strings.TrimSpace(strings.Join(values, "; "))
	}
	return strings.TrimSpace(string(data))
}

type Envelope[T any] struct {
	Status  ResponseStatus  `json:"status"`
	Code    string          `json:"code"`
	Message Message         `json:"message"`
	Data    T               `json:"data"`
	Go      json.RawMessage `json:"go"`
}

func (response Envelope[T]) Successful() bool { return response.Status.Successful() }
