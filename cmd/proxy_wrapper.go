package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type healthCheckProxy struct {
	opts         HealthCheckProxyOptions
	shuttingDown bool
	ctx          context.Context
	log          *slog.Logger
}

type HealthCheckProxyOptions struct {
	RealHealthCheckPath   string
	RealHealthCheckPort   int
	RealHealthCheckScheme string
}

func NewHealthCheckProxy(ctx context.Context, opts HealthCheckProxyOptions) *healthCheckProxy {
	return &healthCheckProxy{
		opts: opts,
		ctx:  ctx,
		log:  slog.Default().With("component", "http-server"),
	}
}

func (h *healthCheckProxy) InitiateShutdown() {
	h.shuttingDown = true
}

func (h *healthCheckProxy) HealthHandler(w http.ResponseWriter, r *http.Request) {
	log := h.log.With("component", "http-health-handler")

	if r.URL.Query().Get("delth.ignoreShuttingDownState") != "1" {
		if h.shuttingDown {
			log.Debug("responding service is shutting down")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "delth: service is shutting down\n")
			return
		}
	}

	if r.Body != nil {
		defer r.Body.Close()
	}

	req, err := http.NewRequestWithContext(h.ctx, r.Method, fmt.Sprintf("%s://localhost:%d%s", h.opts.RealHealthCheckScheme, h.opts.RealHealthCheckPort, h.opts.RealHealthCheckPath), r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Error("creating new HTTP request", "error", err)
		return
	}

	// Remove any query parameter starting with 'delth.'
	// but forward any other query param to backend
	forwadedQueryP := r.URL.Query()
	for qParam := range forwadedQueryP {
		if strings.HasPrefix(qParam, "delth.") {
			forwadedQueryP.Del(qParam)
		}
	}

	req.URL.RawQuery = forwadedQueryP.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		log.Error("performing HTTP request to backend", "error", err)
		return
	}

	defer resp.Body.Close()

	for headerName, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(headerName, value)
		}
	}

	log.Debug("responding with HTTP response from backend", "http-status-code", resp.StatusCode)

	w.WriteHeader(resp.StatusCode)

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Error("copying backend response body to caller", "error", err)
		return
	}
}
