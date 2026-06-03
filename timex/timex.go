package timex

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// Date 日期类型，JSON 序列化为 YYYY-MM-DD 格式。
type Date struct {
	time.Time
}

// DateFormat 定义 Date 的默认解析和序列化格式。
var DateFormat = "2006-01-02"

// SetDateFormat 设置日期格式，空字符串将被忽略。
func SetDateFormat(format string) {
	if format == "" {
		return
	}
	DateFormat = format
}

func (t Date) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte(`null`), nil
	}
	return []byte(`"` + t.Format(DateFormat) + `"`), nil
}

func (t *Date) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		t.Time = time.Time{}
		return nil
	}

	str := string(data[1 : len(data)-1])
	parsed, err := time.Parse(DateFormat, str)
	if err != nil {
		return fmt.Errorf("invalid date format: %w", err)
	}

	t.Time = parsed
	return nil
}

func (t *Date) Scan(value interface{}) error {
	if value == nil {
		t.Time = time.Time{}
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		t.Time = v
	case string:
		parsed, err := time.Parse(DateFormat, v)
		if err != nil {
			return fmt.Errorf("cannot parse date: %w", err)
		}
		t.Time = parsed
	case []byte:
		parsed, err := time.Parse(DateFormat, string(v))
		if err != nil {
			return fmt.Errorf("cannot parse date: %w", err)
		}
		t.Time = parsed
	default:
		return fmt.Errorf("cannot scan %T into Date", value)
	}

	return nil
}

func (t Date) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.Format(DateFormat), nil
}

// DateTime 日期时间类型，JSON 序列化为 YYYY-MM-DD HH:MM:SS 格式。
type DateTime struct {
	time.Time
}

// DateTimeFormat 定义 DateTime 的默认解析和序列化格式。
var DateTimeFormat = "2006-01-02 15:04:05"

// SetDateTimeFormat 设置日期时间格式，空字符串将被忽略。
func SetDateTimeFormat(format string) {
	if format == "" {
		return
	}
	DateTimeFormat = format
}

func (t DateTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte(`null`), nil
	}
	return []byte(`"` + t.Format(DateTimeFormat) + `"`), nil
}

func (t *DateTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		t.Time = time.Time{}
		return nil
	}

	str := string(data[1 : len(data)-1])

	formats := []string{
		DateTimeFormat,
		"2006-01-02T15:04:05",
		time.RFC3339,
		"2006-01-02",
	}

	var parsed time.Time
	var err error

	for _, format := range formats {
		parsed, err = time.Parse(format, str)
		if err == nil {
			t.Time = parsed
			return nil
		}
	}

	return fmt.Errorf("invalid datetime format: %w", err)
}

func (t *DateTime) Scan(value interface{}) error {
	if value == nil {
		t.Time = time.Time{}
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		t.Time = v
	case string:
		parsed, err := time.Parse(DateTimeFormat, v)
		if err != nil {
			parsed, err = time.Parse("2006-01-02 15:04:05.000", v)
			if err != nil {
				parsed, err = time.Parse("2006-01-02T15:04:05Z", v)
			}
			if err != nil {
				return fmt.Errorf("cannot parse datetime: %w", err)
			}
		}
		t.Time = parsed
	case []byte:
		parsed, err := time.Parse(DateTimeFormat, string(v))
		if err != nil {
			return fmt.Errorf("cannot parse datetime: %w", err)
		}
		t.Time = parsed
	default:
		return fmt.Errorf("cannot scan %T into DateTime", value)
	}

	return nil
}

func (t DateTime) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.Format(DateTimeFormat), nil
}

// NewDate 创建日期。
func NewDate(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.Local)}
}

// NewDateTime 创建日期时间。
func NewDateTime(year int, month time.Month, day int, hour, minute, second int) DateTime {
	return DateTime{time.Date(year, month, day, hour, minute, second, 0, time.Local)}
}

// Now 获取当前日期时间。
func Now() DateTime {
	return DateTime{time.Now()}
}

// Today 获取今天的日期。
func Today() Date {
	now := time.Now()
	return Date{time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)}
}
