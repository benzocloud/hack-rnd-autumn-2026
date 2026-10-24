package ml

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

const maxMessageSize = 2 * 1024 * 1024

type client struct {
	rpc     pb.ForecastServiceClient
	timeout time.Duration
}

// New borrows a reusable connection; app owns TLS policy and Close. No connection
// or SQL transaction is created per Predict. Insecure transport is for tests only.
func New(conn *grpc.ClientConn, timeout time.Duration) (*client, error) {
	if conn == nil || timeout <= 0 || timeout > 30*time.Second {
		return nil, forecasting.ErrModelInput
	}
	return &client{rpc: pb.NewForecastServiceClient(conn), timeout: timeout}, nil
}

func (c *client) Predict(ctx context.Context, input forecasting.ModelInput) (forecasting.ModelResult, error) {
	if err := validateInput(input); err != nil {
		return forecasting.ModelResult{}, err
	}
	request := encodeInput(input)
	if proto.Size(request) > maxMessageSize {
		return forecasting.ModelResult{}, forecasting.ErrModelInput
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.rpc.Predict(ctx, request, grpc.MaxCallRecvMsgSize(maxMessageSize), grpc.MaxCallSendMsgSize(maxMessageSize))
	if err != nil {
		return forecasting.ModelResult{}, mapError(err)
	}
	if err := validateResponse(input, response); err != nil {
		return forecasting.ModelResult{}, err
	}
	return decodeResponse(input, response), nil
}
