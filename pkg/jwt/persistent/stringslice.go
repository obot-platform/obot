package persistent

import (
	"encoding/json"
	"strings"
)

type StringSlice []string

func (s StringSlice) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.Join(s, ","))
}

func (s *StringSlice) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	if str == "" {
		*s = nil
		return nil
	}

	*s = StringSlice(strings.Split(str, ","))
	return nil
}
