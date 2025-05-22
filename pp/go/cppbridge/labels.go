package cppbridge

// Labels used for data exchenge between Go and C++
type Labels []Label

// Label is a key/value pair of strings.
type Label struct {
	Name  string
	Value string
}
