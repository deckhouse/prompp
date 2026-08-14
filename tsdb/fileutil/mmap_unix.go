// Copyright 2017 The Prometheus Authors
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

//go:build !windows && !plan9 && !js

package fileutil

import (
	"os"

	"golang.org/x/sys/unix"
)

func mmap(f *os.File, length int) ([]byte, error) {
	b, err := unix.Mmap(int(f.Fd()), 0, length, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}

	// Disable kernel readahead for this mapping. Block index/chunk files are
	// accessed randomly, so the default sequential prefetch only pulls large
	// chunks of the file into the (active) page cache on first touch, inflating
	// the container working set for a long time after a restart.
	_ = unix.Madvise(b, unix.MADV_RANDOM)

	return b, nil
}

func munmap(b []byte) (err error) {
	return unix.Munmap(b)
}
