package state

import "encoding/json"

// JSONCodec implements Codec using JSON encoding
type JSONCodec struct{}

// Encode serializes a value to JSON
func (c *JSONCodec) Encode(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// Decode deserializes JSON data into a value
func (c *JSONCodec) Decode(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// DefaultCodec returns the default JSON codec
func DefaultCodec() Codec {
	return &JSONCodec{}
}
