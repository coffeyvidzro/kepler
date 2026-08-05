package mnotify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type ResponseCode string

func (code *ResponseCode) UnmarshalJSON(data []byte) error {
	if code == nil {
		return fmt.Errorf("%w: response code target is nil", ErrInvalidResponse)
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*code = ""
		return nil
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*code = ResponseCode(strings.TrimSpace(value))
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("mNotify response code must be a string or number: %w", err)
	}
	*code = ResponseCode(number.String())
	return nil
}

func (code ResponseCode) String() string { return strings.TrimSpace(string(code)) }

type Identifier string

func (identifier *Identifier) UnmarshalJSON(data []byte) error {
	if identifier == nil {
		return fmt.Errorf("%w: identifier target is nil", ErrInvalidResponse)
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*identifier = ""
		return nil
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*identifier = Identifier(strings.TrimSpace(value))
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("mNotify identifier must be a string or number: %w", err)
	}
	*identifier = Identifier(number.String())
	return nil
}

func (identifier Identifier) String() string { return strings.TrimSpace(string(identifier)) }

type Response struct {
	Status  string       `json:"status"`
	Code    ResponseCode `json:"code"`
	Message string       `json:"message"`
}

func (response Response) Successful() bool {
	return strings.EqualFold(strings.TrimSpace(response.Status), "success")
}
