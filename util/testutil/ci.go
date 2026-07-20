// Copyright 2026 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutil

import (
	"os"
	"testing"
)

// IsCI reports whether the current process is running inside a CI environment.
// The CI environment variable is set explicitly by our workflow (passed to the
// test container via `docker run -e CI=true`) and also by most CI providers
// (GitHub Actions, GitLab CI, CircleCI, etc.) by convention.
func IsCI() bool {
	return os.Getenv("CI") != ""
}

// SkipIfCI marks t as skipped when running in a CI environment. Use it for
// tests that are too slow or too flaky under heavy CI instrumentation
// (-race + coverage + sanitizers) but still useful when run by a developer
// locally.
func SkipIfCI(t testing.TB, reason string) {
	t.Helper()
	if IsCI() {
		t.Skip("CI-only skip: " + reason)
	}
}
