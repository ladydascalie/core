// Package httpsrv provides a default set of configuration for hosting a http server in a service.
package httpsrv

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/LUSHDigital/core/response"
	"github.com/dustin/go-humanize"
)

const (
	// Port is the default HTTP port.
	Port = 80
)

var (
	// NotFoundHandler responds with the default a 404 response.
	NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		res := &response.Response{
			Code:    http.StatusNotFound,
			Message: http.StatusText(http.StatusNotFound),
		}
		res.WriteTo(w)
	})

	// DefaultHTTPServer represents the default configuration for the http server
	DefaultHTTPServer = http.Server{
		WriteTimeout:      5 * time.Second,
		ReadTimeout:       5 * time.Second,
		IdleTimeout:       5 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
	}
)

// WrapperHandler returns the wrapper handler for the http server.
func WrapperHandler(now func() time.Time, next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/healthz":
			HealthHandler(now)(w, r)
		default:
			next.ServeHTTP(w, r)
		}
	})

}

// HealthHandler responds with service health.
func HealthHandler(now func() time.Time) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := now()

		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		latency := time.Since(start).Nanoseconds() / (1 * 1000 * 1000) // Milliseconds
		res := &response.Response{
			Code:    http.StatusOK,
			Message: http.StatusText(http.StatusOK),
			Data: &response.Data{
				Type: "health",
				Content: HealthResponse{
					Latency:       fmt.Sprintf("%d ms", latency),
					HeapInUse:     humanize.Bytes(mem.HeapInuse),
					HeapAlloc:     humanize.Bytes(mem.HeapAlloc),
					StackInUse:    humanize.Bytes(mem.StackInuse),
					NumGoRoutines: runtime.NumGoroutine(),
				},
			},
		}
		res.WriteTo(w)
	})

}

// NewDefault returns a http server
func NewDefault(handler http.Handler) *Server {
	server := &DefaultHTTPServer
	server.Handler = handler
	return New(server)
}

// New sets up a new HTTP server.
func New(server *http.Server) *Server {
	if server == nil {
		server = &http.Server{}
	}
	if server.WriteTimeout == 0 {
		server.WriteTimeout = DefaultHTTPServer.WriteTimeout
	}
	if server.ReadTimeout == 0 {
		server.ReadTimeout = DefaultHTTPServer.ReadTimeout
	}
	if server.IdleTimeout == 0 {
		server.IdleTimeout = DefaultHTTPServer.IdleTimeout
	}
	if server.ReadHeaderTimeout == 0 {
		server.ReadHeaderTimeout = DefaultHTTPServer.ReadHeaderTimeout
	}

	return &Server{
		Server: server,
		Now:    time.Now,
		addrC:  make(chan *net.TCPAddr, 1),
	}
}

// Server represents a collection of functions for starting and running an RPC server.
type Server struct {
	Server  *http.Server
	Now     func() time.Time
	addrC   chan *net.TCPAddr
	tcpAddr *net.TCPAddr
}

// Run will start the gRPC server and listen for requests.
func (gs *Server) Run(ctx context.Context, out io.Writer) error {
	defer close(gs.addrC)
	addr := gs.Server.Addr
	if addr == "" {
		addr = net.JoinHostPort("", strconv.Itoa(Port))
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	gs.addrC <- lis.Addr().(*net.TCPAddr)

	if gs.Server.Handler == nil {
		return fmt.Errorf("http server needs a handler")
	}

	gs.Server.Handler = WrapperHandler(gs.Now, gs.Server.Handler)

	fmt.Fprintf(out, "serving http on %s", lis.Addr().String())
	return gs.Server.Serve(lis)
}

// Addr will block until you have received an address for your server.
func (gs *Server) Addr() *net.TCPAddr {
	if gs.tcpAddr != nil {
		return gs.tcpAddr
	}
	select {
	case addr := <-gs.addrC:
		return addr
	}
}

// HealthResponse contains information about the service health.
type HealthResponse struct {
	Latency       string `json:"latency"`
	StackInUse    string `json:"stack_in_use"`
	HeapInUse     string `json:"heap_in_use"`
	HeapAlloc     string `json:"heap_alloc"`
	NumGoRoutines int    `json:"num_go_routines"`
}
