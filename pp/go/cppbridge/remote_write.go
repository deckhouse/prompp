package cppbridge

import "runtime"

type RWMessage struct {
	Buffer        []byte
	SampleCount   uint64
	MaxTimestamp  int64
	Delivered     bool
	PostProcessed bool
}

type RWMessageList struct {
	MaxTimestamp int64
	Messages     []RWMessage
}

func NewRWMessageList(messagesCount uint64) *RWMessageList {
	list := &RWMessageList{
		Messages: walRemoteWriteCreateMessages(messagesCount),
	}
	runtime.SetFinalizer(list, func(list *RWMessageList) {
		walRemoteWriteDestroyMessages(list.Messages)
	})

	return list
}

func (m *RWMessageList) HasDataToDeliver() bool {
	for i := range m.Messages {
		if !m.Messages[i].Delivered {
			return true
		}
	}

	return false
}

func (m *RWMessageList) IsObsoleted(minTimestamp int64) bool {
	return m.MaxTimestamp < minTimestamp
}

func (m *RWMessageList) NumberOfSamples() uint64 {
	samples := uint64(0)
	for i := range m.Messages {
		samples += m.Messages[i].SampleCount
	}
	return samples
}

func (m *RWMessageList) UpdateStats() {
	for index := range m.Messages {
		if m.Messages[index].MaxTimestamp > m.MaxTimestamp {
			m.MaxTimestamp = m.Messages[index].MaxTimestamp
		}
	}
}

type MessageEncoders struct {
	encoders    []CppRemoteWriteMessageEncoder
	lssPointers []uintptr
}

func NewMessageEncoders(encodersCount uint64, lssList []*LabelSetStorage) *MessageEncoders {
	encoders := &MessageEncoders{
		encoders:    walRemoteWriteCreateMessageEncoders(encodersCount),
		lssPointers: make([]uintptr, 0, len(lssList)),
	}
	for _, lss := range lssList {
		encoders.lssPointers = append(encoders.lssPointers, lss.Pointer())
	}

	runtime.SetFinalizer(encoders, func(encoders *MessageEncoders) {
		walRemoteWriteDestroyMessageEncoders(encoders.encoders)
	})

	return encoders
}

func (e *MessageEncoders) Encode(
	encoderIndex int,
	batch *SegmentSamplesStorageList,
	messageIndex, messagesCount uint64,
	message *RWMessage,
) {
	walRemoteWriteEncodeMessage(&e.encoders[encoderIndex], e.lssPointers, batch.storages, messageIndex, messagesCount, message)
}
