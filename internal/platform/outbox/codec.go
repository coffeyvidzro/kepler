package outbox

import "encoding/json"

func encodeHeaders(headers map[string]string) ([]byte, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	return json.Marshal(headers)
}

func decodeHeaders(data []byte) (map[string]string, error) {
	headers := map[string]string{}
	if len(data) == 0 {
		return headers, nil
	}
	if err := json.Unmarshal(data, &headers); err != nil {
		return nil, err
	}
	return headers, nil
}
