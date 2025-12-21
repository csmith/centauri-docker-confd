package main

import (
	"encoding/binary"
	"log/slog"
	"net"
	"sync"
)

const (
	magic   = "CENTAURI"
	version = uint32(1)
)

// Server represents a TCP server that broadcasts config to connected clients
type Server struct {
	addr       string
	mu         sync.RWMutex
	clients    map[net.Conn]struct{}
	lastConfig []byte
}

// NewServer creates a new config server
func NewServer(addr string) *Server {
	return &Server{
		addr:    addr,
		clients: make(map[net.Conn]struct{}),
	}
}

// Start starts the TCP server
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	slog.Info("Server listening", "addr", s.addr)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				slog.Error("Failed to accept connection", "err", err)
				continue
			}

			slog.Info("Client connected", "addr", conn.RemoteAddr())

			s.mu.Lock()
			s.clients[conn] = struct{}{}
			lastConfig := s.lastConfig
			s.mu.Unlock()

			if len(lastConfig) > 0 {
				if err := sendConfig(conn, lastConfig); err != nil {
					slog.Error("Failed to send initial config to client", "addr", conn.RemoteAddr(), "err", err)
				} else {
					slog.Info("Sent initial config to client", "addr", conn.RemoteAddr(), "size", len(lastConfig))
				}
			}

			go func(c net.Conn) {
				buf := make([]byte, 1)
				for {
					_, err := c.Read(buf)
					if err != nil {
						slog.Info("Client disconnected", "addr", c.RemoteAddr())
						s.mu.Lock()
						delete(s.clients, c)
						s.mu.Unlock()
						c.Close()
						return
					}
				}
			}(conn)
		}
	}()

	return nil
}

// Broadcast sends the config to all connected clients using the Centauri wire protocol
func (s *Server) Broadcast(config string) {
	payload := []byte(config)

	s.mu.Lock()
	s.lastConfig = payload
	clients := make([]net.Conn, 0, len(s.clients))
	for conn := range s.clients {
		clients = append(clients, conn)
	}
	s.mu.Unlock()

	for _, conn := range clients {
		if err := sendConfig(conn, payload); err != nil {
			slog.Error("Failed to send config to client", "addr", conn.RemoteAddr(), "err", err)
		} else {
			slog.Info("Sent config to client", "addr", conn.RemoteAddr(), "size", len(payload))
		}
	}
}

// sendConfig sends a single config using the Centauri wire protocol
func sendConfig(conn net.Conn, payload []byte) error {
	// Magic bytes
	if _, err := conn.Write([]byte(magic)); err != nil {
		return err
	}

	// Version (4 bytes, big-endian)
	versionBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(versionBytes, version)
	if _, err := conn.Write(versionBytes); err != nil {
		return err
	}

	// Payload length (4 bytes, big-endian)
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(payload)))
	if _, err := conn.Write(lengthBytes); err != nil {
		return err
	}

	// Payload
	if _, err := conn.Write(payload); err != nil {
		return err
	}

	return nil
}
