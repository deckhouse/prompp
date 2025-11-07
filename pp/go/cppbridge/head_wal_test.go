package cppbridge_test

import (
	"testing"

	"github.com/gobuffalo/packr/v2/file/resolver/encoding/hex"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

const hexSegment = "04224D184040C025030000B1D62B2CEB28C11C18DFD92E08008601020000000001000100021200170D1200071700042900150A1200140A2000300000001100FD0A03000000040000000500000006000000070000000800000009320013080400F316190000002100000014000000350000001E000000530000000E0000006100000013000000744E00937A0000001700000091180055A4000000165A00F5AFBA0000002C2975312C3F7C6A3C47343C55453531246E5A673044594B562A3645216E6F3F4D4A6F6D7154273D696C6C5F465765725750304672722F246D3F2E216D4B64313678506C30356C6A4555403F5961284768315F7C35596D713F4D4C293B713D684B786368562F393B593C382E492F597200744A2B244F233B375A32362F703D5F30596E3E3D54235E4A7946675F6267704C5445492E5F66576E704D413C6D28465227784F5E495E64687937776A7462296E52282C43463B4762734A0114016601110D0E00000600E92E3852473464414D6547436F67012500111D0E00000600F90F2262376555745376402A3B4D542A3E6E33243F34242C48716D39792C2302350011170E00000600F90977456B216B512A6C6643244E4E67252C512D3362794B3A032F00110E0E00000600F900474D2D55766D53614B764D6C626D042600111F0E00000600F9116B002A41224D703A3F656A3C4D252B3D6C46246D554E242A623972644C742F05370011180E00000600F90A3C4B356F7724234E7233464B2D38686D5C504763636D495306300011190E00000600F90B65282D795533525F636B6E45693C23702E4749776723503C40073100111B0E00000600F90D764900705C6C32584E4F30725978686975624066253024624C6B21083300111A0E00000600F90C504A7A6377575549593F6D2262672D705E5C21432F2E325F492409320011110E00000600F10464544E562A642D4835534727507500396401019F02D0040901FFFFFFFF683D16B182012D00141C5C03B27D000001773742790135021100F03DAF1C5A643BDFC7FF1C96FFCFE8F5A65B4E3C827DD5EB4DB79C7804BBEE57252B06C1938D78BF329D216BC9D4C8F743B0ACE2AEC78B9C19FAB779E7FBA5CD20B899C395ED280C0901677407941"

//
// HeadWalSuite
//

type HeadWalSuite struct {
	suite.Suite
}

func TestHeadWalSuite(t *testing.T) {
	suite.Run(t, new(HeadWalSuite))
}

func (s *HeadWalSuite) TestHeadWalEncoder_Encode() {
	encoder := cppbridge.NewHeadWalEncoder(0, 1, cppbridge.NewQueryableLssStorage())

	samples, err := encoder.Encode([]*cppbridge.InnerSeries{})
	s.Require().NoError(err)

	s.Empty(samples)
}

func (s *HeadWalSuite) TestHeadWalEncoder_Finalize() {
	encoder := cppbridge.NewHeadWalEncoder(0, 1, cppbridge.NewQueryableLssStorage())

	segmentData, err := encoder.Finalize()
	s.Require().NoError(err)

	s.NotNil(segmentData)
	s.Empty(segmentData.Samples())
}

func (s *HeadWalSuite) TestHeadWalEncoder_EncodeAndFinalize() {
	const kTestBufferVersion = 3

	segment, _ := hex.DecodeString(hexSegment)
	lss := cppbridge.NewQueryableLssStorage()
	decoder := cppbridge.NewHeadWalDecoder(lss, kTestBufferVersion)
	encoder := cppbridge.NewHeadWalEncoder(0, 1, lss)
	innerSeries := cppbridge.NewInnerSeries()
	err := decoder.Decode(segment, innerSeries)
	s.Require().NoError(err)
	s.NotNil(innerSeries)

	expectedSamples := innerSeries.Size()
	samples, err := encoder.Encode([]*cppbridge.InnerSeries{innerSeries})
	s.Require().NoError(err)

	s.NotNil(samples)
	s.Equal(expectedSamples, uint64(samples))

	segmentData, err := encoder.Finalize()
	s.Require().NoError(err)

	s.NotNil(segmentData)
	s.Equal(expectedSamples, uint64(segmentData.Samples()))
}

func TestHeadWalDecoder_DecodeToDataStorage(t *testing.T) {
	// Arrange
	const kTestBufferVersion = 3

	dataStorage := cppbridge.NewDataStorage()
	encoder := cppbridge.NewHeadEncoderWithDataStorage(dataStorage)
	decoder := cppbridge.NewHeadWalDecoder(cppbridge.NewQueryableLssStorage(), kTestBufferVersion)
	segment, _ := hex.DecodeString(hexSegment)

	// Act
	_, _, err := decoder.DecodeToDataStorage(segment, encoder)

	// Assert
	require.NoError(t, err)
	require.Equal(t, cppbridge.TimeInterval{MinT: 1660828401000, MaxT: 1660828410000}, dataStorage.TimeInterval(false))
}
