package goman

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type Hook struct {
	ID        int
	Timestamp string
	Method    string
	Path      string
	Headers   http.Header
	Body      string
	RawBody   string
}

type WebhookServer struct {
	lifecycle    sync.Mutex
	mu           sync.Mutex
	server       *http.Server
	address      string
	hooks        []Hook
	nextID       int
	historyBytes int
	serveErr     error
	OnHook       func(Hook)
}

const MaxHooks = 200
const MaxHistoryBytes = 8 << 20

func (s *WebhookServer) Start(address string) error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return nil
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	s.serveErr = nil
	s.address = listener.Addr().String()
	s.server = &http.Server{Handler: http.HandlerFunc(s.receive), MaxHeaderBytes: 64 << 10, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	server := s.server
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.mu.Lock()
			if s.server == server {
				s.server = nil
				s.serveErr = err
			}
			s.mu.Unlock()
		}
	}()
	return nil
}

func (s *WebhookServer) Address() string { s.mu.Lock(); defer s.mu.Unlock(); return s.address }
func (s *WebhookServer) Running() bool   { s.mu.Lock(); defer s.mu.Unlock(); return s.server != nil }
func (s *WebhookServer) Err() error      { s.mu.Lock(); defer s.mu.Unlock(); return s.serveErr }
func (s *WebhookServer) Hooks() []Hook {
	s.mu.Lock()
	defer s.mu.Unlock()
	hooks := append([]Hook(nil), s.hooks...)
	for index := range hooks {
		hooks[index].Headers = hooks[index].Headers.Clone()
	}
	return hooks
}

func (s *WebhookServer) Stop(ctx context.Context) error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	if err := server.Shutdown(ctx); err != nil {
		server.Close()
		return err
	}
	return nil
}

func (s *WebhookServer) receive(writer http.ResponseWriter, request *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, MaxBodyBytes))
	if err != nil {
		http.Error(writer, "invalid or oversized webhook body", http.StatusRequestEntityTooLarge)
		return
	}
	body, _ := FormatBody(raw)
	if len(raw)+len(body) > MaxHistoryBytes {
		http.Error(writer, "formatted webhook exceeds history budget", http.StatusRequestEntityTooLarge)
		return
	}
	s.mu.Lock()
	s.nextID++
	hook := Hook{ID: s.nextID, Timestamp: time.Now().Format("15:04:05"), Method: request.Method, Path: request.URL.RequestURI(), Headers: request.Header.Clone(), Body: body, RawBody: string(raw)}
	s.hooks = append(s.hooks, hook)
	s.historyBytes += len(hook.RawBody) + len(hook.Body)
	removed := 0
	for len(s.hooks)-removed > 1 && (len(s.hooks)-removed > MaxHooks || s.historyBytes > MaxHistoryBytes) {
		s.historyBytes -= len(s.hooks[removed].RawBody) + len(s.hooks[removed].Body)
		removed++
	}
	if removed > 0 {
		retained := copy(s.hooks, s.hooks[removed:])
		clear(s.hooks[retained:])
		s.hooks = s.hooks[:retained]
	}
	s.mu.Unlock()
	writer.Header().Set("Content-Type", "application/json")
	fmt.Fprint(writer, `{"status":"ok","source":"goman"}`)
	if s.OnHook != nil {
		hook.Headers = hook.Headers.Clone()
		s.OnHook(hook)
	}
}
