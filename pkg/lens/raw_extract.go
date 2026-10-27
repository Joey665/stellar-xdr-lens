package lens

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
)

var (
	// ErrNoRawBytes is returned when the selected node does not correspond to raw bytes.
	ErrNoRawBytes = errors.New("selected node does not contain raw bytes")
)

// ExtractRawBytes extracts the raw bytes from a Value's Raw field at the given path.
// The path uses the same syntax as Node.Find (e.g., "Signatures[0].Hint").
// Returns the raw bytes and the encoding format used ("base64" or "hex").
func ExtractRawBytes(v *Value, path string) ([]byte, string, error) {
	if v == nil || v.Raw == nil {
		return nil, "", ErrNoRawBytes
	}

	// First, try to find the node in the neutral tree
	if v.Node == nil {
		return nil, "", ErrNoRawBytes
	}

	targetNode := v.Node.Find(path)
	if targetNode == nil {
		return nil, "", fmt.Errorf("path %q not found in decoded tree", path)
	}

	// Extract raw bytes from the concrete XDR value using reflection
	rawBytes, encoding, err := extractRawBytesFromReflect(v.Raw, path)
	if err != nil {
		return nil, "", err
	}

	return rawBytes, encoding, nil
}

// extractRawBytesFromReflect uses reflection to navigate to the field and extract raw bytes.
func extractRawBytesFromReflect(root any, path string) ([]byte, string, error) {
	rv := reflect.ValueOf(root)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}

	segs := splitPath(path)
	for _, seg := range segs {
		if rv.Kind() == reflect.Pointer {
			if rv.IsNil() {
				return nil, "", ErrNoRawBytes
			}
			rv = rv.Elem()
		}

		if seg.name != "" {
			if rv.Kind() != reflect.Struct {
				return nil, "", ErrNoRawBytes
			}
			rv = rv.FieldByName(seg.name)
			if !rv.IsValid() {
				return nil, "", fmt.Errorf("field %q not found", seg.name)
			}
		}

		if seg.index >= 0 {
			if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
				return nil, "", ErrNoRawBytes
			}
			if seg.index >= rv.Len() {
				return nil, "", fmt.Errorf("index %d out of bounds (len=%d)", seg.index, rv.Len())
			}
			rv = rv.Index(seg.index)
		}
	}

	// Now rv should point to the target field. Extract raw bytes.
	return extractBytesFromValue(rv)
}

// extractBytesFromValue extracts raw bytes from a reflect.Value.
// Handles: []byte, [32]byte, xdr.Hash, xdr.PublicKey, xdr.Signature, etc.
func extractBytesFromValue(rv reflect.Value) ([]byte, string, error) {
	// Unwrap pointers
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, "", ErrNoRawBytes
		}
		rv = rv.Elem()
	}

	if !rv.IsValid() {
		return nil, "", ErrNoRawBytes
	}

	// Handle []byte
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
		if rv.IsNil() {
			return nil, "", ErrNoRawBytes
		}
		return rv.Bytes(), "base64", nil
	}

	// Handle [N]byte arrays
	if rv.Kind() == reflect.Array && rv.Type().Elem().Kind() == reflect.Uint8 {
		n := rv.Len()
		buf := make([]byte, n)
		for i := 0; i < n; i++ {
			buf[i] = byte(rv.Index(i).Uint())
		}
		return buf, "hex", nil
	}

	// Handle specific XDR types that are byte arrays
	typeName := rv.Type().Name()
	switch typeName {
	case "Hash", "PublicKey", "Signature", "ContractId", "PoolId", "Mac":
		// These are [32]byte or similar
		if rv.Kind() == reflect.Array && rv.Type().Elem().Kind() == reflect.Uint8 {
			n := rv.Len()
			buf := make([]byte, n)
			for i := 0; i < n; i++ {
				buf[i] = byte(rv.Index(i).Uint())
			}
			return buf, "hex", nil
		}
	case "Uint256":
		// [32]byte
		if rv.Kind() == reflect.Array && rv.Type().Elem().Kind() == reflect.Uint8 {
			n := rv.Len()
			buf := make([]byte, n)
			for i := 0; i < n; i++ {
				buf[i] = byte(rv.Index(i).Uint())
			}
			return buf, "hex", nil
		}
	}

	// If the value has a MarshalBinary method, try that
	if marshaller, ok := rv.Interface().(interface{ MarshalBinary() ([]byte, error) }); ok {
		data, err := marshaller.MarshalBinary()
		if err == nil && len(data) > 0 {
			return data, "base64", nil
		}
	}

	// For scalars that were rendered as hex in the node, try to parse from the node's Value
	if rv.Kind() == reflect.String {
		s := rv.String()
		// Try base64 first
		if data, err := base64.StdEncoding.DecodeString(s); err == nil {
			return data, "base64", nil
		}
		// Try hex
		if data, err := hex.DecodeString(s); err == nil {
			return data, "hex", nil
		}
	}

	return nil, "", ErrNoRawBytes
}

// ExtractRawBytesFromNode extracts raw bytes from a Node's Value field if it represents bytes.
func ExtractRawBytesFromNode(n *Node) ([]byte, string, error) {
	if n == nil {
		return nil, "", ErrNoRawBytes
	}

	// If the node's Value is a hex string (from byte array rendering), decode it
	if n.Kind == KindScalar && n.Value != nil {
		if s, ok := n.Value.(string); ok {
			// Try to decode as hex first (since byte arrays render as hex)
			if data, err := hex.DecodeString(s); err == nil {
				return data, "hex", nil
			}
			// Try base64
			if data, err := base64.StdEncoding.DecodeString(s); err == nil {
				return data, "base64", nil
			}
		}
		// If it's already []byte or a byte slice type
		if b, ok := n.Value.([]byte); ok {
			return b, "base64", nil
		}
		// Handle named byte slice types (like xdr.Signature, xdr.Hash, etc.)
		rv := reflect.ValueOf(n.Value)
		if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
			return rv.Bytes(), "base64", nil
		}
		if rv.Kind() == reflect.Array && rv.Type().Elem().Kind() == reflect.Uint8 {
			n := rv.Len()
			buf := make([]byte, n)
			for i := 0; i < n; i++ {
				buf[i] = byte(rv.Index(i).Uint())
			}
			return buf, "hex", nil
		}
	}

	return nil, "", ErrNoRawBytes
}
