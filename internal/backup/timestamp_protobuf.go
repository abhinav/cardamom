package backup

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func timestampToProto(name string, value time.Time) (*timestamppb.Timestamp, error) {
	if value.IsZero() {
		return nil, fmt.Errorf("%s is required", name)
	}
	encoded := timestamppb.New(value)
	if err := encoded.CheckValid(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}
	return encoded, nil
}

func optionalTimestampToProto(
	name string,
	value *time.Time,
) (*timestamppb.Timestamp, error) {
	if value == nil {
		return nil, nil
	}
	return timestampToProto(name, *value)
}

func timestampFromProto(
	name string,
	value *timestamppb.Timestamp,
) (time.Time, error) {
	if value == nil {
		return time.Time{}, fmt.Errorf("%s is required", name)
	}
	if err := value.CheckValid(); err != nil {
		return time.Time{}, fmt.Errorf("invalid %s: %w", name, err)
	}
	return value.AsTime(), nil
}

func optionalTimestampFromProto(
	name string,
	value *timestamppb.Timestamp,
) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	decoded, err := timestampFromProto(name, value)
	if err != nil {
		return nil, err
	}
	return &decoded, nil
}
