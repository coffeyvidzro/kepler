package messagetemplate

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

var placeholderPattern = regexp.MustCompile(`\{\{\{\s*([A-Za-z][A-Za-z0-9_]*)\s*\}\}\}`)

func Render(version Version, supplied map[string]any) (PreviewResponse, error) {
	values := make(map[string]string, len(version.Variables))
	definitions := make(map[string]Variable, len(version.Variables))
	for _, variable := range version.Variables {
		definitions[variable.Key] = variable
		value, exists := supplied[variable.Key]
		if !exists {
			value = variable.FallbackValue
		}
		if value == nil {
			continue
		}
		normalized, err := renderVariableValue(variable, value)
		if err != nil {
			return PreviewResponse{}, err
		}
		values[variable.Key] = normalized
	}

	render := func(input string, escape bool) (string, error) {
		var renderErr error
		output := placeholderPattern.ReplaceAllStringFunc(input, func(match string) string {
			parts := placeholderPattern.FindStringSubmatch(match)
			key := parts[1]
			if _, ok := definitions[key]; !ok {
				renderErr = fmt.Errorf("unknown template variable %s", key)
				return match
			}
			value, ok := values[key]
			if !ok {
				renderErr = fmt.Errorf("template variable %s is required", key)
				return match
			}
			if escape {
				return html.EscapeString(value)
			}
			return value
		})
		return output, renderErr
	}

	subject, err := render(version.Subject, false)
	if err != nil {
		return PreviewResponse{}, err
	}
	htmlBody, err := render(version.HTML, true)
	if err != nil {
		return PreviewResponse{}, err
	}
	var textBody *string
	if version.Text != nil {
		value, renderErr := render(*version.Text, false)
		if renderErr != nil {
			return PreviewResponse{}, renderErr
		}
		textBody = &value
	}
	return PreviewResponse{
		TemplateID: version.TemplateID, VersionID: version.ID,
		Subject: subject, HTML: htmlBody, Text: textBody,
		FromEmail: version.FromEmail, FromName: version.FromName, ReplyTo: version.ReplyToEmail,
	}, nil
}

func renderVariableValue(variable Variable, value any) (string, error) {
	switch variable.Type {
	case VariableTypeString:
		text, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("template variable %s must be a string", variable.Key)
		}
		return text, nil
	case VariableTypeNumber:
		switch number := value.(type) {
		case json.Number:
			if _, err := number.Float64(); err != nil {
				return "", fmt.Errorf("template variable %s must be a number", variable.Key)
			}
			return number.String(), nil
		case float64:
			return strconv.FormatFloat(number, 'f', -1, 64), nil
		case float32:
			return strconv.FormatFloat(float64(number), 'f', -1, 64), nil
		case int:
			return strconv.Itoa(number), nil
		case int32:
			return strconv.FormatInt(int64(number), 10), nil
		case int64:
			return strconv.FormatInt(number, 10), nil
		default:
			return "", fmt.Errorf("template variable %s must be a number", variable.Key)
		}
	default:
		return "", fmt.Errorf("unsupported template variable type %q", variable.Type)
	}
}

func referencedVariables(inputs ...string) []string {
	set := map[string]struct{}{}
	for _, input := range inputs {
		for _, match := range placeholderPattern.FindAllStringSubmatch(input, -1) {
			set[strings.TrimSpace(match[1])] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	return result
}
