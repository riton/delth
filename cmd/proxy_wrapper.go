/*
Copyright © 2024 Rémi Ferrand

Contributor(s): Rémi Ferrand <riton.github_at_gmail.com>, 2024

This software is governed by the CeCILL license under French law and
abiding by the rules of distribution of free software.  You can  use,
modify and/ or redistribute the software under the terms of the CeCILL
license as circulated by CEA, CNRS and INRIA at the following URL
"http://www.cecill.info".

As a counterpart to the access to the source code and  rights to copy,
modify and redistribute granted by the license, users are provided only
with a limited warranty  and the software's author,  the holder of the
economic rights,  and the successive licensors  have only  limited
liability.

In this respect, the user's attention is drawn to the risks associated
with loading,  using,  modifying and/or developing or reproducing the
software by the user in light of its specific status of free software,
that may mean  that it is complicated to manipulate,  and  that  also
therefore means  that it is reserved for developers  and  experienced
professionals having in-depth computer knowledge. Users are therefore
encouraged to load and test the software's suitability as regards their
requirements in conditions enabling the security of their systems and/or
data to be ensured and,  more generally, to use and operate it in the
same conditions as regards security.

The fact that you are presently reading this means that you have had
knowledge of the CeCILL license and that you accept its terms.
*/
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
	hClient      httpDoer
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
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

func (h *healthCheckProxy) SetHTTPClient(c httpDoer) {
	h.hClient = c
}

func (h *healthCheckProxy) getHTTPClient() httpDoer {
	if h.hClient == nil {
		return http.DefaultClient
	}

	return h.hClient
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

	resp, err := h.getHTTPClient().Do(req)
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
