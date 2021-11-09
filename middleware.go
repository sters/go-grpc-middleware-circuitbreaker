package middleware

import (
	"context"

	"github.com/mercari/go-circuitbreaker"
	"google.golang.org/grpc"
)

type OpenStateHandler func(ctx context.Context, method string, req interface{})

func UnaryClientInterceptor(cb *circuitbreaker.CircuitBreaker, handler OpenStateHandler) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		_, err := cb.Do(ctx, func() (interface{}, error) {
			err := invoker(ctx, method, req, reply, cc, opts...)
			if err != nil {
				return nil, err
			}

			return nil, nil
		})

		if err == circuitbreaker.ErrOpen {
			handler(ctx, method, req)
		}

		return err
	}
}
