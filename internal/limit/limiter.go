package limit

import (
	"sync"

	"golang.org/x/time/rate"
)

// IPLimiter controls the rate of requests per IP address.
type IPLimiter struct {
	mu  sync.Mutex
	ips map[string]*rate.Limiter
	r   rate.Limit
	b   int
}

// NewIPLimiter returns a new IPLimiter with the given rate and burst.
func NewIPLimiter(r rate.Limit, b int) *IPLimiter {
	return &IPLimiter{
		ips: make(map[string]*rate.Limiter),
		r:   r,
		b:   b,
	}
}

// Allow checks if the request from the given IP is allowed.
func (l *IPLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	limiter, exists := l.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(l.r, l.b)
		l.ips[ip] = limiter
	}

	return limiter.Allow()
}

// ConnLimiter tracks and limits the number of active connections.
type ConnLimiter struct {
	mu         sync.Mutex
	ipCounts   map[string]int
	totalCount int
	maxPerIP   int
	maxGlobal  int
}

// NewConnLimiter returns a new ConnLimiter with per-IP and global limits.
func NewConnLimiter(maxPerIP, maxGlobal int) *ConnLimiter {
	return &ConnLimiter{
		ipCounts:  make(map[string]int),
		maxPerIP:  maxPerIP,
		maxGlobal: maxGlobal,
	}
}

// Increment increments the connection count for the given IP.
// Returns true if the connection is allowed, false otherwise.
func (l *ConnLimiter) Increment(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.totalCount >= l.maxGlobal {
		return false
	}

	if l.ipCounts[ip] >= l.maxPerIP {
		return false
	}

	l.ipCounts[ip]++
	l.totalCount++
	return true
}

// Decrement decrements the connection count for the given IP.
func (l *ConnLimiter) Decrement(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.ipCounts[ip] > 0 {
		l.ipCounts[ip]--
		if l.ipCounts[ip] == 0 {
			delete(l.ipCounts, ip)
		}
		l.totalCount--
	}
}
