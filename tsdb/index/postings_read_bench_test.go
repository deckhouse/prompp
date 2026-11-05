package index

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage"
)

// BenchmarkPostingsIntersectionRead includes both in-memory and on-disk postings encodings.
func BenchmarkPostingsIntersectionRead(b *testing.B) {
	for _, pattern := range []string{"aligned", "offset", "sparse_first", "sparse_last", "disjoint"} {
		refs, expected := postingsReadFixture(pattern)
		encoded := make([][]byte, len(refs))

		for i, values := range refs {
			encoded[i] = make([]byte, 4*len(values))

			for j, value := range values {
				binary.BigEndian.PutUint32(encoded[i][4*j:], uint32(value)) // #nosec G115 // fixture IDs fit uint32
			}
		}

		b.Run(pattern, func(b *testing.B) {
			for _, encoding := range []string{"list", "big_endian"} {
				b.Run(encoding, func(b *testing.B) {
					for _, seek := range []bool{false, true} {
						b.Run(fmt.Sprintf("seek=%t", seek), func(b *testing.B) {
							postings := make([]Postings, len(refs))
							var count int
							b.ReportAllocs()
							b.ResetTimer()

							for b.Loop() {
								for i := range postings {
									if encoding == "list" {
										postings[i] = NewListPostings(refs[i])
									} else {
										postings[i] = newBigEndianPostings(encoded[i])
									}
								}

								intersection := Intersect(postings...)
								count = 0
								var ok bool

								if seek {
									ok = intersection.Seek(1)
								} else {
									ok = intersection.Next()
								}

								for ok {
									count++
									ok = intersection.Next()
								}

								if err := intersection.Err(); err != nil {
									b.Fatal(err)
								}
							}

							b.StopTimer()
							require.Equal(b, expected, count)
						})
					}
				})
			}
		})
	}
}

func postingsReadFixture(pattern string) ([][]storage.SeriesRef, int) {
	const size = 32_768
	refs := make([][]storage.SeriesRef, 3)

	for id := storage.SeriesRef(1); id <= size; id++ {
		switch pattern {
		case "aligned":
			refs[0] = append(refs[0], id)
			refs[1] = append(refs[1], id)
			refs[2] = append(refs[2], id)
		case "offset":
			if id%2 == 0 {
				refs[0] = append(refs[0], id)
			}

			if id%3 == 0 {
				refs[1] = append(refs[1], id)
			}

			if id%5 == 0 {
				refs[2] = append(refs[2], id)
			}
		case "sparse_first", "sparse_last", "disjoint":
			if id%2 == 0 {
				refs[0] = append(refs[0], id)
			} else {
				refs[1] = append(refs[1], id)
			}
		default:
			panic("unknown postings pattern: " + pattern)
		}
	}

	switch pattern {
	case "aligned":
		return refs, size
	case "offset":
		return refs, size / 30
	case "sparse_first", "sparse_last":
		refs[1] = append(refs[1], size)
		refs[2] = []storage.SeriesRef{size}

		if pattern == "sparse_first" {
			refs[0], refs[2] = refs[2], refs[0]
		}

		return refs, 1
	default:
		refs[2] = []storage.SeriesRef{size}

		return refs, 0
	}
}
