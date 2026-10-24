package core

import (
	"context"
	"errors"
	"io"
	"time"

	"google.golang.org/grpc"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

// UploadFile forwards measured bytes without retrying an outcome-unknown write.
// The caller owns reader. The child context always closes the stream on failure.
func UploadFile(ctx context.Context, client corev1.ImportsServiceClient, metadata *corev1.UploadImportMetadata, reader io.Reader) (*corev1.UploadImportResponse, error) {
	if metadata == nil || metadata.ByteSize == 0 || metadata.ByteSize > 100<<20 {
		return nil, errors.New("invalid upload metadata")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	stream, err := client.UploadImport(ctx, grpc.MaxCallSendMsgSize(2<<20), grpc.MaxCallRecvMsgSize(2<<20))
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&corev1.UploadImportRequest{Frame: &corev1.UploadImportRequest_Metadata{Metadata: metadata}}); err != nil {
		return uploadSendError(stream, err)
	}
	buffer := make([]byte, 256<<10)
	var sent uint64
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := reader.Read(buffer)
		if n > 0 {
			sent += uint64(n)
			if sent > metadata.ByteSize {
				return nil, errors.New("upload size changed")
			}
			if err := stream.Send(&corev1.UploadImportRequest{Frame: &corev1.UploadImportRequest_Chunk{Chunk: append([]byte(nil), buffer[:n]...)}}); err != nil {
				return uploadSendError(stream, err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
	if sent != metadata.ByteSize {
		return nil, io.ErrUnexpectedEOF
	}
	return stream.CloseAndRecv()
}

func uploadSendError(stream grpc.ClientStreamingClient[corev1.UploadImportRequest, corev1.UploadImportResponse], err error) (*corev1.UploadImportResponse, error) {
	if errors.Is(err, io.EOF) {
		return stream.CloseAndRecv()
	}
	return nil, err
}
