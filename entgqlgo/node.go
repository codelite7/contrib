// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package entgqlgo

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// GlobalID encodes a type name and local ID into a globally unique ID.
// The format is base64(TypeName:LocalID).
func GlobalID(typeName string, id interface{}) string {
	raw := fmt.Sprintf("%s:%v", typeName, id)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// ParseGlobalID decodes a global ID back into its type name and local ID.
// Returns an error if the ID is malformed.
func ParseGlobalID(globalID string) (typeName string, id string, err error) {
	decoded, err := base64.StdEncoding.DecodeString(globalID)
	if err != nil {
		// Try as raw string (unencoded) for backward compatibility
		decoded = []byte(globalID)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid global ID format: %q", globalID)
	}
	return parts[0], parts[1], nil
}

// ParseGlobalIDInt decodes a global ID and returns the local ID as an int.
// This is a convenience function for entities with integer IDs.
func ParseGlobalIDInt(globalID string) (typeName string, id int, err error) {
	typeName, idStr, err := ParseGlobalID(globalID)
	if err != nil {
		return "", 0, err
	}
	id, err = strconv.Atoi(idStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid integer ID in global ID: %w", err)
	}
	return typeName, id, nil
}

// MustGlobalID is like GlobalID but panics if there's an error.
// This is useful for tests and initialization code.
func MustGlobalID(typeName string, id interface{}) string {
	return GlobalID(typeName, id)
}
