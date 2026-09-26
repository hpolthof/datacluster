package handlers

import (
	"io"
	"net"
	"net/http"
)

// Relay handles incoming TCP tunnel requests from other DataCluster instances.
// It upgrades the HTTP connection and bidirectionally pipes to the target host:port
// on the local Docker network.
func (h *Handlers) Relay(w http.ResponseWriter, r *http.Request) {
	targetHost := r.URL.Query().Get("host")
	targetPort := r.URL.Query().Get("port")
	if targetHost == "" || targetPort == "" {
		writeError(w, http.StatusBadRequest, "host and port are required")
		return
	}

	target := net.JoinHostPort(targetHost, targetPort)
	dst, err := net.Dial("tcp", target)
	if err != nil {
		http.Error(w, "relay: cannot reach target: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer dst.Close()

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "relay: hijacking not supported by server", http.StatusInternalServerError)
		return
	}

	src, buf, err := hj.Hijack()
	if err != nil {
		dst.Close()
		return
	}
	defer src.Close()

	src.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: tcp-tunnel\r\nConnection: Upgrade\r\n\r\n"))

	// Flush any bytes the HTTP server buffered before hijacking
	if buf.Reader.Buffered() > 0 {
		buffered := make([]byte, buf.Reader.Buffered())
		buf.Reader.Read(buffered)
		dst.Write(buffered)
	}

	done := make(chan struct{}, 2)
	go func() { io.Copy(dst, src); done <- struct{}{} }()
	go func() { io.Copy(src, dst); done <- struct{}{} }()
	<-done
}
