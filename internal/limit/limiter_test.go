package limit

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestIPLimiter(t *testing.T) {
	// Allow 2 requests per second with burst of 2
	limiter := NewIPLimiter(rate.Limit(2), 2)

	ip := "192.168.1.1"

	// First 2 requests should be allowed (burst)
	if !limiter.Allow(ip) {
		t.Error("Request 1 should be allowed")
	}
	if !limiter.Allow(ip) {
		t.Error("Request 2 should be allowed")
	}

	// Third request should be blocked (exceeds burst and rate)
	if limiter.Allow(ip) {
		t.Error("Request 3 should be blocked")
	}

	// Wait for refill
	time.Sleep(600 * time.Millisecond)

	// Should be allowed again
	if !limiter.Allow(ip) {
		t.Error("Request 4 should be allowed after wait")
	}
}

func TestConnLimiter_PerIP(t *testing.T) {
	// Max 2 connections per IP, 10 global
	limiter := NewConnLimiter(2, 10)
	ip := "10.0.0.1"

	if !limiter.Increment(ip) {
		t.Error("First connection should be allowed")
	}
	if !limiter.Increment(ip) {
		t.Error("Second connection should be allowed")
	}
	if limiter.Increment(ip) {
		t.Error("Third connection should be rejected")
	}

	limiter.Decrement(ip)
	if !limiter.Increment(ip) {
		t.Error("Connection should be allowed after decrement")
	}
}

func TestConnLimiter_Global(t *testing.T) {
	// Max 10 per IP, 2 global
	limiter := NewConnLimiter(10, 2)
	ip1 := "10.0.0.1"
	ip2 := "10.0.0.2"

	if !limiter.Increment(ip1) {
		t.Error("First global connection should be allowed")
	}
	if !limiter.Increment(ip2) {
		t.Error("Second global connection should be allowed")
	}
	if limiter.Increment("10.0.0.3") {
		t.Error("Third global connection should be rejected")
	}

	limiter.Decrement(ip1)
	if !limiter.Increment("10.0.0.3") {
		t.Error("Connection should be allowed after global decrement")
	}
}
