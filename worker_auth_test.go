package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackRemoteAddr(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{addr: "127.0.0.1:1234", want: true},
		{addr: "[::1]:8080", want: true},
		{addr: "localhost:9000", want: true},
		{addr: "8.8.8.8:53", want: false},
	}

	for _, tt := range tests {
		if got := isLoopbackRemoteAddr(tt.addr); got != tt.want {
			t.Fatalf("isLoopbackRemoteAddr(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestAuthorizeRequest_NoTokenAllowsOnlyLoopback(t *testing.T) {
	ws := &WorkerServer{}

	loopbackReq := httptest.NewRequest(http.MethodPost, "/download", nil)
	loopbackReq.RemoteAddr = "127.0.0.1:10000"
	loopbackResp := httptest.NewRecorder()
	if ok := ws.authorizeRequest(loopbackResp, loopbackReq); !ok {
		t.Fatalf("loopback request should be authorized")
	}

	remoteReq := httptest.NewRequest(http.MethodPost, "/download", nil)
	remoteReq.RemoteAddr = "8.8.8.8:10000"
	remoteResp := httptest.NewRecorder()
	if ok := ws.authorizeRequest(remoteResp, remoteReq); ok {
		t.Fatalf("remote request without token should be rejected")
	}
	if remoteResp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", remoteResp.Code, http.StatusForbidden)
	}
}

func TestAuthorizeRequest_WithTokenRequiresHeader(t *testing.T) {
	ws := &WorkerServer{workerAPIToken: "secret-token"}

	noHeaderReq := httptest.NewRequest(http.MethodPost, "/download", nil)
	noHeaderReq.RemoteAddr = "127.0.0.1:10000"
	noHeaderResp := httptest.NewRecorder()
	if ok := ws.authorizeRequest(noHeaderResp, noHeaderReq); ok {
		t.Fatalf("request without token header should be rejected")
	}
	if noHeaderResp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", noHeaderResp.Code, http.StatusUnauthorized)
	}

	okReq := httptest.NewRequest(http.MethodPost, "/download", nil)
	okReq.RemoteAddr = "8.8.8.8:10000"
	okReq.Header.Set(workerAuthHeader, "secret-token")
	okResp := httptest.NewRecorder()
	if ok := ws.authorizeRequest(okResp, okReq); !ok {
		t.Fatalf("request with correct token header should be authorized")
	}
}
