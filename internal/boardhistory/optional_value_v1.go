package boardhistory

import (
	"errors"
	"time"
)

// V1 field assignments distinguish clearing a field from assigning its zero
// value. These records retain that distinction for board and issue events.
type optionalTextV1 struct {
	Present bool   `json:"present"`
	Value   string `json:"value"`
}

func optionalTextToV1(value *string) optionalTextV1 {
	if value == nil {
		return optionalTextV1{}
	}
	return optionalTextV1{Present: true, Value: *value}
}

func (v *optionalTextV1) toText() (*string, error) {
	if !v.Present {
		if v.Value != "" {
			return nil, errors.New("absent text assignment must not contain a value")
		}
		return nil, nil
	}
	return new(v.Value), nil
}

type optionalInt64V1 struct {
	Present bool  `json:"present"`
	Value   int64 `json:"value"`
}

func optionalInt64ToV1(value *int64) optionalInt64V1 {
	if value == nil {
		return optionalInt64V1{}
	}
	return optionalInt64V1{Present: true, Value: *value}
}

func (v *optionalInt64V1) toInt64() (*int64, error) {
	if !v.Present {
		if v.Value != 0 {
			return nil, errors.New("absent integer assignment must not contain a value")
		}
		return nil, nil
	}
	return new(v.Value), nil
}

type optionalTimeV1 struct {
	Present bool      `json:"present"`
	Value   time.Time `json:"value"`
}

func optionalTimeToV1(value *time.Time) optionalTimeV1 {
	if value == nil {
		return optionalTimeV1{}
	}
	return optionalTimeV1{Present: true, Value: value.UTC()}
}

func (v *optionalTimeV1) toTime() (*time.Time, error) {
	if v.Present == v.Value.IsZero() {
		return nil, errors.New("time assignment has inconsistent presence")
	}
	if !v.Present {
		return nil, nil
	}
	return new(v.Value), nil
}

func timePointerUTC(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return new(value.UTC())
}
