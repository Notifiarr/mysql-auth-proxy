package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/userinfo"
)

// req is our request (input channel).
type req struct {
	ctx context.Context //nolint:containedctx
	key string
	del bool
}

// res is our response (output channel).
type res struct {
	*UserInfo
	error
}

func (c *Cache) start() {
	c.users = make(map[string]*UserInfo)
	c.req = make(chan *req)
	c.res = make(chan *res)

	go c.processRequests()
}

func (c *Cache) stop() {
	close(c.req)
	<-c.res
	c.req = nil
	c.res = nil
}

// clean it up.
func (c *Cache) clean() {
	for k := range c.users {
		c.users[k] = nil
		delete(c.users, k)
	}

	c.users = nil
}

// respond offloads the response payload.
func (c *Cache) respond(userInfo *UserInfo, err error) {
	if userInfo == nil {
		userInfo = &UserInfo{UserInfo: *userinfo.DefaultUser(), Cached: time.Now()}
	}

	c.res <- &res{UserInfo: userInfo, error: err}
}

// processRequests is the main running go routine for the cache.
func (c *Cache) processRequests() {
	defer close(c.res) // close response channel when request channel closes.

	for req := range c.req {
		if req.del {
			c.respond(c.deleteRequest(req.key), nil)
		} else {
			c.respond(c.users[req.key], c.userRequest(req.ctx, req.key))
		}
	}
}

// deleteRequest is used when a key is deleted or the envionrmnet is changed.
func (c *Cache) deleteRequest(requestKey string) *UserInfo {
	userInfo := c.users[requestKey].copy() // copy values to return them.
	c.users[requestKey] = nil              // free memory.
	delete(c.users, requestKey)            // make it go bye bye.

	return userInfo
}

// copy is use to save values before deleting them.
func (u *UserInfo) copy() *UserInfo {
	if u == nil {
		return &UserInfo{UserInfo: *userinfo.DefaultUser(), Cached: time.Now()}
	}

	return &UserInfo{UserInfo: *u.UserInfo.Copy(), Cached: u.Cached}
}

// userRequest checks for cache, and sets a user's info if it's not cached.
func (c *Cache) userRequest(ctx context.Context, requestKey string) error {
	if _, exists := c.users[requestKey]; exists {
		return nil
	}

	userInfo, err := c.ui.GetInfo(ctx, requestKey)
	if err != nil {
		return fmt.Errorf("getting user info: %w", err)
	}

	c.users[requestKey] = &UserInfo{UserInfo: *userInfo, Cached: time.Now()}

	return nil
}
