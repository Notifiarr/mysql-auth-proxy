package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
)

// Cache provides methods to get a user and delete a user from cache.
// If the user is not in cache it is fetched using the userinfo module.
type Cache struct {
	ui    *userinfo.UI
	users map[string]*UserInfo
	req   chan *req
	res   chan *res
}

// UserInfo contains a user's info, and a cached bool flag.
type UserInfo struct {
	userinfo.UserInfo
	Cached time.Time
}

// New starts the cache routine and returns a struct to get data from the cache.
func New(config *userinfo.Config) (*Cache, error) {
	ui, err := userinfo.New(config)
	if err != nil {
		return nil, fmt.Errorf("initializing userinfo: %w", err)
	}

	cache := &Cache{ui: ui}
	cache.start()

	return cache, nil
}

// Starts sets up the cache and starts the go routine.
func (c *Cache) Start() error {
	if c.req != nil {
		return nil
	}

	if err := c.ui.Open(); err != nil {
		return fmt.Errorf("initializing userinfo: %w", err)
	}

	c.clean()
	c.start()

	return nil
}

// Stop stops the go routine and closes the channels.
// If clean is true it will clean up memory usage.
// Pass clean if the app will continue to run.
func (c *Cache) Stop(clean bool) {
	c.ui.Close()
	c.stop()

	if clean {
		c.clean()
	}
}

// GetUserInfo returns a user's info from cache or from the user database.
func (c *Cache) GetUserInfo(ctx context.Context, requestKey string) (*UserInfo, error) {
	c.req <- &req{key: requestKey, ctx: ctx}
	ret := <-c.res

	return ret.UserInfo, ret.error
}

// DelCacheKey removes a cached key and it's data.
func (c *Cache) DelCacheKey(requestKey string) *UserInfo {
	c.req <- &req{key: requestKey, del: true}
	ret := <-c.res

	return ret.UserInfo
}
