package store

import (
	"database/sql/driver"
	"encoding/csv"
	"fmt"
	"strings"
)

type StringArray []string

func (s *StringArray) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	switch v := src.(type) {
	case []string:
		*s = v
		return nil
	case string:
		return s.parseTextArray(v)
	case []byte:
		return s.parseTextArray(string(v))
	default:
		return fmt.Errorf("unsupported type %T", src)
	}
}

func (s StringArray) Value() (driver.Value, error) {
	return driver.Value([]string(s)), nil
}

func (s *StringArray) parseTextArray(input string) error {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || trimmed == "{}" {
		*s = []string{}
		return nil
	}
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		trimmed = strings.TrimPrefix(trimmed, "{")
		trimmed = strings.TrimSuffix(trimmed, "}")
	}

	r := csv.NewReader(strings.NewReader(trimmed))
	r.Comma = ','
	r.LazyQuotes = true
	record, err := r.Read()
	if err != nil {
		return err
	}
	for i, item := range record {
		record[i] = strings.Trim(item, "\"")
	}
	*s = record
	return nil
}
