package model

import "github.com/prometheus/prometheus/pp/go/model"

//
// ProtobufData
//

// ProtobufData is an universal interface for blob protobuf data.
type ProtobufData interface {
	Bytes() []byte
	Destroy()
}

//
// TimeSeriesBatch
//

// TimeSeriesBatch is an universal interface for batch [model.TimeSeries].
type TimeSeriesBatch interface {
	TimeSeries() []model.TimeSeries
	Destroy()
}
