package middleware

import (
	"context"
	"mime"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/prometheus/prometheus/op-pkg/handler/model"
)

type metadataCtxKey struct{}

func ContextWithMetadata(ctx context.Context, metadata model.Metadata) context.Context {
	return context.WithValue(ctx, metadataCtxKey{}, metadata)
}

func MetadataFromContext(ctx context.Context) model.Metadata {
	return ctx.Value(metadataCtxKey{}).(model.Metadata)
}

func ResolveMetadata(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		agentUUIDString := request.Header.Get("X-Agent-UUID")
		if agentUUIDString == "" {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		agentUUID, err := uuid.Parse(agentUUIDString)
		if err != nil {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		productName := request.Header.Get("User-Agent")
		agentHostname := request.Header.Get("X-Agent-Hostname")

		blockIDString := request.Header.Get("X-Block-ID")
		if blockIDString == "" {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		blockID, err := uuid.Parse(blockIDString)
		if err != nil {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		shardIDStr := request.Header.Get("X-Shard-ID")
		if shardIDStr == "" {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		//revive:disable-next-line:add-constant not need const
		shardID, err := strconv.ParseUint(shardIDStr, 10, 0)
		if err != nil {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		shardsLogStr := request.Header.Get("X-Shards-Log")
		if shardsLogStr == "" {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		//revive:disable-next-line:add-constant not need const
		shardsLog, err := strconv.ParseUint(shardsLogStr, 10, 0)
		if err != nil {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		contentType := request.Header.Get("Content-Type")
		if contentType == "" {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		mediaType, param, err := mime.ParseMediaType(contentType)
		if err != nil {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		protocolVersionString, ok := param["version"]
		if !ok {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		//revive:disable-next-line:add-constant not need const
		protocolVersion, err := strconv.ParseUint(protocolVersionString, 10, 0)
		if err != nil {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		seversionStr, ok := param["segment_encoding_version"]
		if !ok {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		//revive:disable-next-line:add-constant not need const
		segmentEncodingVersion, err := strconv.ParseUint(seversionStr, 10, 0)
		if err != nil {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		metadata := model.Metadata{
			BlockID:                blockID,
			ShardID:                uint16(shardID),
			ShardsLog:              uint8(shardsLog),
			SegmentEncodingVersion: uint8(segmentEncodingVersion),
			ProtocolVersion:        uint8(protocolVersion),
			ProductName:            productName,
			AgentHostname:          agentHostname,
			AgentUUID:              agentUUID,
			MediaType:              mediaType,
		}

		next(writer, request.WithContext(ContextWithMetadata(request.Context(), metadata)))
	}
}
