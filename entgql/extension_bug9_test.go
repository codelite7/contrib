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

package entgql_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBug9_TodoFixtureBuilds is the integration test that drives the fix for
// Bug 9 in the entgql collection-subpkg dispatch plan
// (docs/superpowers/plans/2026-05-17-entgql-collection-subpkg-dispatch.md).
//
// Bug 9: when ent's PR 6 (per-entity sub-packages) is in effect, the gen-root
// `ent` package exposes types like `*<Entity>Query` as Go type aliases to the
// per-entity sub-package's concrete `*Query`. entgql's WithSplitGoFiles(true)
// emits `gen/gql_collection_<entity>.go` and similar files that declare methods
// on those `*<Entity>Query` aliases inside the `ent` package — Go forbids that
// ("cannot define new methods on non-local type"). The same diagnostic surfaces
// on aliased enum types (e.g. `Status`) that entgql decorates with
// MarshalGQL/UnmarshalGQL methods.
//
// The test runs `go build` against the entgql/internal/todo fixture and asserts
// that the Bug 9 diagnostic ("cannot define new methods on non-local type") is
// present. The test is expected to FAIL today (Task 1 baseline) and SHOULD PASS
// after Tasks 2-7 of the plan land.
//
// `go test` sets the test process's working dir to the test package's dir
// (`entgql/`), so we set Dir to ".." (the contrib repo root) before running
// `go build`.
//
// The build target is `./entgql/internal/todo/ent/...` (the gen layer + its
// per-entity sub-packages) rather than the broader `./entgql/internal/todo/...`.
// The broader target also tries to compile the application-layer resolvers,
// which currently trip on an unrelated "use of internal package" error after
// PR 6's split — that error short-circuits the build before any of the Bug 9
// sites are checked, masking the diagnostic this test is designed to catch.
// Bug 9 lives entirely in the gen layer, so scoping the build there surfaces
// it cleanly.
func TestBug9_TodoFixtureBuilds(t *testing.T) {
	cmd := exec.Command("go", "build", "./entgql/internal/todo/ent/...")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("go build output:\n%s", out)
		if strings.Contains(string(out), "cannot define new methods on non-local type") {
			t.Fatal("Bug 9 active: cannot define new methods on non-local type. " +
				"Tasks 2-7 of the implementation plan should fix this.")
		}
		t.Fatalf("unexpected build failure: %v\n%s", err, out)
	}
}
