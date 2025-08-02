package valueobject

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math/big"
)

type BigFloat struct {
	*big.Float
}

//
// 👇 JSON SUPPORT
//

// Encode as JSON number, not string
func (bf BigFloat) MarshalJSON() ([]byte, error) {
	if bf.Float == nil {
		return []byte("null"), nil
	}

	// Convert *big.Float to float64 (⚠️ may lose precision)
	f64, _ := bf.Float.Float64()
	return json.Marshal(f64)
}

// UnmarshalJSON accepts both number and string (e.g. 123 or "123.45")
func (bf *BigFloat) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		bf.Float = nil
		return nil
	}

	var str string
	// Handle number (not in quotes)
	if data[0] == '"' {
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
	} else {
		str = string(data)
	}

	f, _, err := big.ParseFloat(str, 10, 256, big.ToNearestEven)
	if err != nil {
		return fmt.Errorf("invalid big.Float value: %v", err)
	}
	bf.Float = f
	return nil
}

//
// 👇 DATABASE SUPPORT
//

// Value converts BigFloat to driver.Value for INSERT/UPDATE
func (bf BigFloat) Value() (driver.Value, error) {
	if bf.Float == nil {
		return nil, nil
	}
	return bf.Text('f', -1), nil // use string form
}

// Scan converts SQL value into BigFloat for SELECT
func (bf *BigFloat) Scan(value interface{}) error {
	switch v := value.(type) {
	case string:
		return bf.UnmarshalJSON([]byte(`"` + v + `"`))
	case []byte:
		return bf.UnmarshalJSON([]byte(`"` + string(v) + `"`))
	case nil:
		bf.Float = nil
		return nil
	default:
		return fmt.Errorf("unsupported type for BigFloat Scan: %T", value)
	}
}
