package server

import (
	"pawiu-db/types"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
)

var c *cache.Cache
var config types.Config

func init() {
	c = cache.New(5*time.Minute, 10*time.Minute)
}

// read
func getCache(key string) (interface{}, bool) {
	if !config.Cache_incoming_all && !config.Cache_outgoing_all {
		return nil, false
	}
	cachedData, found := c.Get(key)
	return cachedData, found
}

func saveCache(key string, data interface{}) bool {
	duration := time.Duration(config.Cache_incoming_time) * time.Second
	c.Set(key, data, duration)

	if strings.Contains(key, ".") {
		parts := strings.Split(key, ".")
		baseKey := parts[0]
		c.Delete(baseKey)
	}

	return true
}

// save
func cacheIncoming(key string, data interface{}) bool {
	// if config.Cache_incoming_all {
	duration := time.Duration(config.Cache_incoming_time) * time.Second
	c.Set(key, data, duration)
	return true
	// }
	return false
}

// save
func cacheOutgoing(key string, data interface{}) bool {
	// if config.Cache_outgoing_all {
	duration := time.Duration(config.Cache_outgoing_time) * time.Second
	c.Set(key, data, duration)
	return true
	// }
	return false
}
