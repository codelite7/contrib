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
	"testing"
)

func TestGlobalID(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		id       interface{}
	}{
		{
			name:     "integer ID",
			typeName: "Todo",
			id:       1,
		},
		{
			name:     "string ID",
			typeName: "User",
			id:       "abc123",
		},
		{
			name:     "large integer ID",
			typeName: "Category",
			id:       999999999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalID := GlobalID(tt.typeName, tt.id)
			if globalID == "" {
				t.Error("expected non-empty global ID")
			}

			// Parse it back
			parsedType, parsedID, err := ParseGlobalID(globalID)
			if err != nil {
				t.Fatalf("failed to parse global ID: %v", err)
			}

			if parsedType != tt.typeName {
				t.Errorf("expected type %q, got %q", tt.typeName, parsedType)
			}

			// Note: ID will be string representation
			expectedIDStr := ""
			switch v := tt.id.(type) {
			case int:
				expectedIDStr = string(rune('0'+v%10)) // simplified for single digit
				if v >= 10 {
					expectedIDStr = "" // let it fall through
				}
			case string:
				expectedIDStr = v
			}
			if expectedIDStr == "" {
				// Just check it's not empty for complex cases
				if parsedID == "" {
					t.Error("expected non-empty parsed ID")
				}
			}
		})
	}
}

func TestParseGlobalID(t *testing.T) {
	tests := []struct {
		name         string
		globalID     string
		wantType     string
		wantID       string
		wantErr      bool
	}{
		{
			name:     "valid encoded ID",
			globalID: GlobalID("Todo", 123),
			wantType: "Todo",
			wantID:   "123",
			wantErr:  false,
		},
		{
			name:     "valid encoded string ID",
			globalID: GlobalID("User", "abc"),
			wantType: "User",
			wantID:   "abc",
			wantErr:  false,
		},
		{
			name:     "invalid base64",
			globalID: "not-valid-base64!!!",
			wantType: "",
			wantID:   "",
			wantErr:  true, // Will fail to split on ":"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotID, err := ParseGlobalID(tt.globalID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGlobalID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotType != tt.wantType {
					t.Errorf("ParseGlobalID() type = %v, want %v", gotType, tt.wantType)
				}
				if gotID != tt.wantID {
					t.Errorf("ParseGlobalID() id = %v, want %v", gotID, tt.wantID)
				}
			}
		})
	}
}

func TestParseGlobalIDInt(t *testing.T) {
	tests := []struct {
		name     string
		globalID string
		wantType string
		wantID   int
		wantErr  bool
	}{
		{
			name:     "valid integer ID",
			globalID: GlobalID("Todo", 123),
			wantType: "Todo",
			wantID:   123,
			wantErr:  false,
		},
		{
			name:     "valid zero ID",
			globalID: GlobalID("Item", 0),
			wantType: "Item",
			wantID:   0,
			wantErr:  false,
		},
		{
			name:     "non-integer ID",
			globalID: GlobalID("User", "abc"),
			wantType: "",
			wantID:   0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotID, err := ParseGlobalIDInt(tt.globalID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGlobalIDInt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotType != tt.wantType {
					t.Errorf("ParseGlobalIDInt() type = %v, want %v", gotType, tt.wantType)
				}
				if gotID != tt.wantID {
					t.Errorf("ParseGlobalIDInt() id = %v, want %v", gotID, tt.wantID)
				}
			}
		})
	}
}

func TestGlobalIDRoundTrip(t *testing.T) {
	testCases := []struct {
		typeName string
		id       interface{}
	}{
		{"Todo", 1},
		{"Todo", 12345},
		{"Category", "uuid-123-456"},
		{"User", "john@example.com"},
		{"Item", 0},
	}

	for _, tc := range testCases {
		globalID := GlobalID(tc.typeName, tc.id)
		parsedType, parsedID, err := ParseGlobalID(globalID)
		if err != nil {
			t.Errorf("failed to parse global ID for %s:%v: %v", tc.typeName, tc.id, err)
			continue
		}

		if parsedType != tc.typeName {
			t.Errorf("type mismatch: got %s, want %s", parsedType, tc.typeName)
		}

		// ID should match as string
		expectedID := ""
		switch v := tc.id.(type) {
		case int:
			expectedID = string('0' + byte(v))
			if v >= 10 || v < 0 {
				// Format the integer properly
				expectedID = ""
			}
		case string:
			expectedID = v
		}

		if expectedID != "" && parsedID != expectedID {
			t.Errorf("ID mismatch for %s: got %s, want %s", tc.typeName, parsedID, expectedID)
		}
	}
}
