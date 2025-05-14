package cppbridge_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type LabelsSuite struct {
	suite.Suite
}

func TestLabelsSuite(t *testing.T) {
	suite.Run(t, new(LabelsSuite))
}

func (s *LabelsSuite) TestString() {
	expected := "{t1=\"t1\", t2=\"t2\"}"

	actual := cppbridge.FromMap(map[string]string{
		"t1": "t1",
		"t2": "t2",
	})

	s.Equal(expected, actual.String())
}

func (s *LabelsSuite) TestStringEmpty() {
	expected := "{}"

	actual := cppbridge.FromMap(map[string]string{})

	s.Equal(expected, actual.String())
}

func (s *LabelsSuite) TestIsValid() {
	for _, test := range []struct {
		input    cppbridge.Labels
		expected bool
	}{
		{
			input: cppbridge.FromMap(map[string]string{
				"__name__": "test",
				"hostname": "localhost",
				"job":      "check",
			}),
			expected: true,
		},
		{
			input: cppbridge.FromMap(map[string]string{
				"__name__":     "test:ms",
				"hostname_123": "localhost",
				"_job":         "check",
			}),
			expected: true,
		},
		{
			input:    cppbridge.FromMap(map[string]string{"__name__": "test-ms"}),
			expected: false,
		},
		{
			input:    cppbridge.FromMap(map[string]string{"__name__": "0zz"}),
			expected: false,
		},
		{
			input:    cppbridge.FromMap(map[string]string{"abc:xyz": "invalid"}),
			expected: false,
		},
		{
			input:    cppbridge.FromMap(map[string]string{"123abc": "invalid"}),
			expected: false,
		},
		{
			input:    cppbridge.FromMap(map[string]string{"中文abc": "invalid"}),
			expected: false,
		},
		{
			input:    cppbridge.FromMap(map[string]string{"invalid": "aa\xe2"}),
			expected: false,
		},
		{
			input:    cppbridge.FromMap(map[string]string{"invalid": "\xF7\xBF\xBF\xBF"}),
			expected: false,
		},
	} {
		s.Run("", func() {
			s.Require().Equal(test.expected, test.input.IsValid())
		})
	}
}

func (s *LabelsSuite) TestJSON() {
	lbls := cppbridge.FromMap(map[string]string{
		"aaa": "111",
		"bbb": "2222",
		"ccc": "33333",
	})

	expectedJSON := "{\"aaa\":\"111\",\"bbb\":\"2222\",\"ccc\":\"33333\"}"

	b, err := json.Marshal(lbls)
	s.Require().NoError(err)
	s.Require().Equal(expectedJSON, string(b))

	var gotJ cppbridge.Labels
	err = json.Unmarshal(b, &gotJ)
	s.Require().NoError(err)
	s.Require().Equal(lbls, gotJ)

	// Now in a struct with a tag
	type foo struct {
		ALabels cppbridge.Labels `json:"a_labels,omitempty"`
	}

	expectedJSONFromStruct := "{\"a_labels\":" + expectedJSON + "}"

	f := foo{ALabels: lbls}
	b, err = json.Marshal(f)
	s.Require().NoError(err)
	s.Require().Equal(expectedJSONFromStruct, string(b))

	var gotFJ foo
	err = json.Unmarshal(b, &gotFJ)
	s.Require().NoError(err)
	s.Require().Equal(f, gotFJ)
}

func (s *LabelsSuite) TestYAML() {
	lbls := cppbridge.FromMap(map[string]string{
		"aaa": "111",
		"bbb": "2222",
		"ccc": "33333",
	})

	expectedYAML := "aaa: \"111\"\nbbb: \"2222\"\nccc: \"33333\"\n"
	b, err := yaml.Marshal(lbls)
	s.Require().NoError(err)
	s.Require().Equal(expectedYAML, string(b))

	var gotY cppbridge.Labels
	err = yaml.Unmarshal(b, &gotY)
	s.Require().NoError(err)
	s.Require().Equal(lbls, gotY)

	// Now in a struct with a tag
	type foo struct {
		ALabels cppbridge.Labels `yaml:"a_labels,omitempty"`
	}

	f := foo{ALabels: lbls}

	expectedYAMLFromStruct := "a_labels:\n  aaa: \"111\"\n  bbb: \"2222\"\n  ccc: \"33333\"\n"

	b, err = yaml.Marshal(f)
	s.Require().NoError(err)
	s.Require().Equal(expectedYAMLFromStruct, string(b))

	var gotFY foo
	err = yaml.Unmarshal(b, &gotFY)
	s.Require().NoError(err)
	s.Require().Equal(f, gotFY)
}

func (s *LabelsSuite) TestEqual() {
	lsA := cppbridge.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := cppbridge.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	s.True(cppbridge.Equal(lsA, lsB))
}

func (s *LabelsSuite) TestEqualEmpty() {
	lsA := cppbridge.EmptyLabels()

	lsB := cppbridge.EmptyLabels()

	s.True(cppbridge.Equal(lsA, lsB))
}

func (s *LabelsSuite) TestNotEqualOnLen() {
	lsA := cppbridge.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := cppbridge.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"imya":     "reck",
	})

	s.False(cppbridge.Equal(lsA, lsB))
}

func (s *LabelsSuite) TestNotEqualOnLabel() {
	lsA := cppbridge.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := cppbridge.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"imya":     "reck",
	})

	s.False(cppbridge.Equal(lsA, lsB))
}

func (s *LabelsSuite) TestNotEqualOnEmpty() {
	lsA := cppbridge.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := cppbridge.EmptyLabels()

	s.False(cppbridge.Equal(lsA, lsB))
}
