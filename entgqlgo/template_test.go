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

	"github.com/stretchr/testify/assert"
)

func TestTrimPrefix_EnumValueStripping(t *testing.T) {
	// The enum template uses gqlgoTrimPrefix to strip the type prefix
	// from enum Go constant names (e.g., "PhysicalOfficeATLANTA" -> "ATLANTA").
	// This matches gqlgen convention where enum values don't include the type prefix.
	tests := []struct {
		name     string
		enumName string // $e.Name: the Go constant name
		prefix   string // pascal($f.Name): the pascal-cased field name
		want     string // expected stripped enum value
	}{
		{
			name:     "strips pascal field prefix from uppercase value",
			enumName: "PhysicalOfficeATLANTA",
			prefix:   "PhysicalOffice",
			want:     "ATLANTA",
		},
		{
			name:     "strips pascal field prefix from mixed case value",
			enumName: "PhysicalOfficeCHICAGO",
			prefix:   "PhysicalOffice",
			want:     "CHICAGO",
		},
		{
			name:     "strips pascal field prefix with underscore value",
			enumName: "PhysicalOfficeEL_SEGUNDO",
			prefix:   "PhysicalOffice",
			want:     "EL_SEGUNDO",
		},
		{
			name:     "strips simple field prefix",
			enumName: "StatusACTIVE",
			prefix:   "Status",
			want:     "ACTIVE",
		},
		{
			name:     "no-op when prefix does not match",
			enumName: "ACTIVE",
			prefix:   "Status",
			want:     "ACTIVE",
		},
		{
			name:     "strips multi-word field prefix",
			enumName: "AccountTypeINDIVIDUAL",
			prefix:   "AccountType",
			want:     "INDIVIDUAL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimPrefix(tt.enumName, tt.prefix)
			assert.Equal(t, tt.want, got, "trimPrefix(%q, %q)", tt.enumName, tt.prefix)
		})
	}
}
