package middleware

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/mercari/go-circuitbreaker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	pb "google.golang.org/grpc/examples/helloworld/helloworld"
	"google.golang.org/grpc/status"
)

type server struct {
	pb.UnimplementedGreeterServer

	duringError int
	counter     int32
}

var _ pb.GreeterServer = (*server)(nil)

func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	atomic.AddInt32(&s.counter, 1)
	fmt.Println("[Server] Request handled.")

	if n := s.duringError - int(atomic.LoadInt32(&s.counter)); n >= 0 {
		return nil, status.New(codes.Internal, fmt.Sprintf("fail %d times left", n)).Err()
	}

	return &pb.HelloReply{Message: "Hello " + in.Name}, nil
}

func ExampleUnaryClientInterceptor() {
	lnAddress := "127.0.0.1:13000"

	// server
	go func() {
		listener, _ := net.Listen("tcp", lnAddress)
		defer listener.Close()

		s := grpc.NewServer()
		defer s.Stop()

		pb.RegisterGreeterServer(s, &server{
			duringError: 5,
		})

		_ = s.Serve(listener)
	}()

	// client
	go func() {
		time.Sleep(2 * time.Second) // waiting for serve
		ctx := context.Background()

		cb := circuitbreaker.New(
			circuitbreaker.WithCounterResetInterval(time.Minute),
			circuitbreaker.WithTripFunc(circuitbreaker.NewTripFuncThreshold(3)),
			circuitbreaker.WithOpenTimeout(2500*time.Millisecond),
			circuitbreaker.WithHalfOpenMaxSuccesses(3),
		)

		client, _ := grpc.DialContext(
			ctx,
			lnAddress,
			grpc.WithInsecure(),
			grpc.WithUnaryInterceptor(
				grpc_middleware.ChainUnaryClient(
					UnaryClientInterceptor(
						cb,
						func(ctx context.Context, method string, req interface{}) {
							fmt.Printf("[Client] Circuit breaker is open.\n")
						},
					),
				),
			),
		)
		defer client.Close()

		greeterClient := pb.NewGreeterClient(client)

		timer := time.NewTicker(time.Second)
		for {
			select {
			case <-timer.C:
				time.Sleep(50 * time.Millisecond)
				response, err := greeterClient.SayHello(ctx, &pb.HelloRequest{Name: "foo"})
				fmt.Printf("[Client] Response = %v, cb.State() = %v, Err = %v\n", response, cb.State(), err)
			default:
			}
		}
	}()

	time.Sleep(20 * time.Second)

	// Output:
	// [Server] Request handled.
	// [Client] Response = <nil>, cb.State() = closed, Err = rpc error: code = Internal desc = fail 4 times left
	// [Server] Request handled.
	// [Client] Response = <nil>, cb.State() = closed, Err = rpc error: code = Internal desc = fail 3 times left
	// [Server] Request handled.
	// [Client] Response = <nil>, cb.State() = open, Err = rpc error: code = Internal desc = fail 2 times left
	// [Client] Circuit breaker is open.
	// [Client] Response = <nil>, cb.State() = open, Err = circuit breaker open
	// [Client] Circuit breaker is open.
	// [Client] Response = <nil>, cb.State() = open, Err = circuit breaker open
	// [Server] Request handled.
	// [Client] Response = <nil>, cb.State() = open, Err = rpc error: code = Internal desc = fail 1 times left
	// [Client] Circuit breaker is open.
	// [Client] Response = <nil>, cb.State() = open, Err = circuit breaker open
	// [Client] Circuit breaker is open.
	// [Client] Response = <nil>, cb.State() = open, Err = circuit breaker open
	// [Server] Request handled.
	// [Client] Response = <nil>, cb.State() = open, Err = rpc error: code = Internal desc = fail 0 times left
	// [Client] Circuit breaker is open.
	// [Client] Response = <nil>, cb.State() = open, Err = circuit breaker open
	// [Client] Circuit breaker is open.
	// [Client] Response = <nil>, cb.State() = open, Err = circuit breaker open
	// [Server] Request handled.
	// [Client] Response = message:"Hello foo", cb.State() = half-open, Err = <nil>
	// [Server] Request handled.
	// [Client] Response = message:"Hello foo", cb.State() = half-open, Err = <nil>
	// [Server] Request handled.
	// [Client] Response = message:"Hello foo", cb.State() = closed, Err = <nil>
	// [Server] Request handled.
	// [Client] Response = message:"Hello foo", cb.State() = closed, Err = <nil>
	// [Server] Request handled.
	// [Client] Response = message:"Hello foo", cb.State() = closed, Err = <nil>
	// [Server] Request handled.
	// [Client] Response = message:"Hello foo", cb.State() = closed, Err = <nil>
}
