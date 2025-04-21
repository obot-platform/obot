package types

import "time"

type Time struct {
	Time time.Time
}

func (t *Time) IsZero() bool {
	return t == nil || t.Time.IsZero()
}

func (t *Time) GetTime() time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.Time
}

// NewTimeFromPointer creates a new Time object from a pointer to a time.Time object. If the pointer is nil, the
// function returns nil.
func NewTimeFromPointer(t *time.Time) *Time {
	if t == nil {
		return nil
	}
	return NewTime(*t)
}

func NewTime(t time.Time) *Time {
	if t.IsZero() {
		return &Time{}
	}
	return &Time{Time: t}
}

// DeepCopyInto creates a deep-copy of the Time value. You can do a straight copy of the object because the
// underlying time.Time type is effectively immutable in the time API.
func (t *Time) DeepCopyInto(out *Time) {
	*out = *t
}

func (t *Time) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	return t.Time.UnmarshalJSON(data)
}

func (t Time) MarshalJSON() ([]byte, error) {
	if t.Time.IsZero() {
		return []byte("null"), nil
	}
	return t.Time.MarshalJSON()
}

// ToUnstructured implement value.UnstructuredConverter to make k8s happy? Dunno if I really need this.
func (t Time) ToUnstructured() interface{} {
	if t.Time.IsZero() {
		return nil
	}
	buf := make([]byte, 0, len(time.RFC3339))
	buf = t.Time.UTC().AppendFormat(buf, time.RFC3339)
	return string(buf)
}

func (Time) OpenAPISchemaType() []string {
	return []string{"string"}
}

func (Time) OpenAPISchemaFormat() string {
	return "date-time"
}
