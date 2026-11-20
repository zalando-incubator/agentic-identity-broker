package consent

import "net/url"

func sessionTokenParameter(query url.Values) (string, error) {
	values, present := query["session_token"]
	if !present {
		return "", nil
	}
	if len(values) != 1 || values[0] == "" {
		return "", errInvalidToken
	}
	return values[0], nil
}
