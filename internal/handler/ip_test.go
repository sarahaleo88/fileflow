package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetClientIP(t *testing.T) {
	// Setup Trusted Proxies
	SetTrustedProxies([]string{"10.0.0.0/8", "127.0.0.1", "::1"})

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		wantIP     string
	}{
		{
			name:       "Direct Connection (Untrusted)",
			remoteAddr: "203.0.113.1:12345",
			headers:    nil,
			wantIP:     "203.0.113.1",
		},
		{
			name:       "Trusted Proxy with XFF",
			remoteAddr: "127.0.0.1:55555",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.5, 10.0.0.1",
			},
			wantIP: "203.0.113.5", // 10.0.0.1 is trusted, skip it. 203.0.113.5 is untrusted.
		},
		{
			name:       "Trusted Proxy with Spoofed XFF",
			remoteAddr: "127.0.0.1:55555",
			headers: map[string]string{
				"X-Forwarded-For": "spoofed-ip, 203.0.113.5, 10.0.0.1",
			},
			wantIP: "203.0.113.5", // 10.0.0.1 trusted. 203.0.113.5 untrusted (real client).
		},
		{
			name:       "Untrusted Proxy with XFF (Ignored)",
			remoteAddr: "192.0.2.1:44444", // Not in trusted list
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.5",
			},
			wantIP: "192.0.2.1", // Ignore XFF, take RemoteAddr
		},
		{
			name:       "IPv6 RemoteAddr",
			remoteAddr: "[::1]:12345", // Trusted (localhost)
			headers: map[string]string{
				"X-Forwarded-For": "2001:db8::1",
			},
			wantIP: "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := getClientIP(req)
			if got != tt.wantIP {
				t.Errorf("getClientIP() = %q, want %q", got, tt.wantIP)
			}
		})
	}
}
