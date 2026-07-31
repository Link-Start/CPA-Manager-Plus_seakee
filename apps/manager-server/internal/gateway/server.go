package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

type Server struct {
	servers   []*http.Server
	listeners []net.Listener
}

func Listen(addrs []string, handler http.Handler) (*Server, error) {
	if len(addrs) == 0 {
		return nil, errors.New("at least one gateway listen address is required")
	}
	listeners := make([]net.Listener, 0, len(addrs))
	servers := make([]*http.Server, 0, len(addrs))
	for _, addr := range addrs {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			for _, opened := range listeners {
				_ = opened.Close()
			}
			return nil, fmt.Errorf("listen on gateway address %s: %w", addr, err)
		}
		listeners = append(listeners, listener)
		servers = append(servers, &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		})
	}
	return &Server{servers: servers, listeners: listeners}, nil
}

func Serve(ctx context.Context, addrs []string, handler http.Handler) error {
	server, err := Listen(addrs, handler)
	if err != nil {
		return err
	}
	return server.Serve(ctx)
}

func (s *Server) Serve(ctx context.Context) error {
	if s == nil || len(s.servers) == 0 || len(s.servers) != len(s.listeners) {
		return errors.New("gateway server is not initialized")
	}

	result := make(chan error, len(s.servers))
	for index := range s.servers {
		server := s.servers[index]
		listener := s.listeners[index]
		go func() {
			err := server.Serve(listener)
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			result <- err
		}()
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, server := range s.servers {
			_ = server.Shutdown(shutdownCtx)
		}
		return nil
	case err := <-result:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, server := range s.servers {
			_ = server.Shutdown(shutdownCtx)
		}
		return err
	}
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var firstErr error
	for _, listener := range s.listeners {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
