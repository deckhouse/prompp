package cppbridge

import "runtime"

type RWMessage struct {
	samplesIterator CppSegmentSamplesStorageListIterator
	Buffer          []byte
	MaxTimestamp    int64
	SampleCount     uint32
	Delivered       bool
	PostProcessed   bool
}

type RWMessageList struct {
	MaxTimestamp    int64
	TargetSegmentID uint32
	Messages        []RWMessage
}

func NewRWMessageList(targetSegmentID uint32, messages []RWMessage) *RWMessageList {
	list := &RWMessageList{
		TargetSegmentID: targetSegmentID,
		Messages:        messages,
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
		samples += uint64(m.Messages[i].SampleCount)
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

func NewMessageEncoders(encodersCount uint64, lssList []*LabelSetSnapshot) *MessageEncoders {
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
	messageIndex, messagesCount uint64,
	messages []RWMessage,
) {
	walRemoteWriteEncodeMessage(&e.encoders[encoderIndex], e.lssPointers, messageIndex, messagesCount, messages)
	runtime.KeepAlive(e)
	runtime.KeepAlive(messages)
}
