package middlewares

import (
	"net/http"
	"sync"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/server/logger"

	"github.com/gin-gonic/gin"
)

type ipInfo struct {
	count     int
	lastSeen  time.Time
	warnCount int
	blocked   bool
	blockTime time.Time
	resetTime time.Time
}

type RateLimiterConfig struct {
	RequestLimit     int
	WindowMinutes    int
	WarningThreshold int
	BlockTimeout     int
}

var (
	ips             = make(map[string]map[string]*ipInfo)
	mu              sync.RWMutex
	cleanupInterval = 30 * time.Minute
	maxInactiveTime = 2 * time.Hour
)

func RateLimiter(config RateLimiterConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		route := c.FullPath()

		mu.Lock()
		defer mu.Unlock()

		if _, exists := ips[ip]; !exists {
			ips[ip] = make(map[string]*ipInfo)
		}

		info, exists := ips[ip][route]
		if !exists {
			info = &ipInfo{
				resetTime: time.Now().Add(time.Duration(config.WindowMinutes) * time.Minute),
			}
			ips[ip][route] = info
		}

		if time.Now().After(info.resetTime) {
			info.count = 0
			info.warnCount = 0
			info.blocked = false
			info.resetTime = time.Now().Add(time.Duration(config.WindowMinutes) * time.Minute)
		}

		if info.blocked {
			if time.Since(info.blockTime) < time.Duration(config.BlockTimeout)*time.Minute {
				respondWithBlock(c, ip, route, config.BlockTimeout)
				return
			}
			info.blocked = false
			info.count = 0
			info.warnCount = 0
		}

		if info.count >= config.RequestLimit {
			info.warnCount++
			if info.warnCount >= config.WarningThreshold {
				info.blocked = true
				info.blockTime = time.Now()
				respondWithBlock(c, ip, route, config.BlockTimeout)
				return
			}
			respondWithWarning(c, ip, route, info.warnCount)
			return
		}

		info.count++
		info.lastSeen = time.Now()
		c.Next()
	}
}

func respondWithBlock(c *gin.Context, ip, route string, timeout int) {
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error":   "Blocked for excessive requests",
		"timeout": timeout,
	})
	logger.Log("WARN", "IP blocked for route", []logger.LogDetail{
		{Key: "IP", Value: ip},
		{Key: "Route", Value: route},
		{Key: "Timeout", Value: timeout},
	})
}

func respondWithWarning(c *gin.Context, ip, route string, warnCount int) {
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error": "Too many requests - warning issued",
		"warns": warnCount,
	})
	logger.Log("WARN", "Too many requests for route", []logger.LogDetail{
		{Key: "IP", Value: ip},
		{Key: "Route", Value: route},
		{Key: "Warns", Value: warnCount},
	})
}

func cleanup() {
	for {
		time.Sleep(cleanupInterval)
		mu.Lock()
		for ip, routes := range ips {
			for route, info := range routes {
				if time.Since(info.lastSeen) > maxInactiveTime {
					delete(routes, route)
				}
			}
			if len(routes) == 0 {
				delete(ips, ip)
			}
		}
		mu.Unlock()
	}
}

func init() {
	go cleanup()
}
