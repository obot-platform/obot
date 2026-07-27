package persistent

import (
	"encoding/json"
	"time"
)

type Time struct {
	time.Time
}

func NewTime(t time.Time) Time {
	return Time{t}
}

func (t Time) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Unix())
}

func (t *Time) UnmarshalJSON(data []byte) error {
	var unix int64
	if err := json.Unmarshal(data, &unix); err != nil {
		return err
	}
	t.Time = time.Unix(unix, 0)
	return nil
}
