package server

import (
	"pawiu-db/types"
	"time"

	"github.com/patrickmn/go-cache"
)

var c *cache.Cache
var config types.Config

func init() {
	c = cache.New(5*time.Minute, 10*time.Minute)
}

func getCache(key string) (interface{}, bool) {
	if !config.Cache_incoming_all && !config.Cache_outgoing_all {
		return nil, false
	}
	cachedData, found := c.Get(key)
	return cachedData, found
}

func cacheIncoming(key string, data interface{}) bool {
	if config.Cache_incoming_all {
		duration := time.Duration(config.Cache_incoming_time) * time.Second
		c.Set(key, data, duration)
		return true
	}
	return false
}

func cacheOutgoing(key string, data interface{}) bool {
	if config.Cache_outgoing_all {
		duration := time.Duration(config.Cache_outgoing_time) * time.Second
		c.Set(key, data, duration)
		return true
	}
	return false
}
