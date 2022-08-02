package cache

import (
	"time"
)

// Cache provides methods to get a user and delete a user from cache.
// If the user is not in cache it is fetched using the userinfo module.
type Cache struct {
	cache map[string]*Item
	req   chan *req
	res   chan *res
}

// Item is what's returned from a cache Get.
type Item struct {
	Data interface{} `json:"data"`
	Time time.Time   `json:"created"`
}

// New starts the cache routine and returns a struct to get data from the cache.
func New() *Cache {
	cache := &Cache{}
	cache.start()

	return cache
}

// Starts sets up the cache and starts the go routine.
func (c *Cache) Start() {
	c.clean()
	c.start()
}

// Stop stops the go routine and closes the channels.
// If clean is true it will clean up memory usage.
// Pass clean if the app will continue to run.
func (c *Cache) Stop(clean bool) {
	c.stop()

	if clean {
		c.clean()
	}
}

// Get returns an item, or nil if it doesn't exist.
func (c *Cache) Get(requestKey string) *Item {
	c.req <- &req{key: requestKey}
	ret := <-c.res

	return ret.item
}

// Save saves an item, and returns true if it already existed.
func (c *Cache) Save(requestKey string, data interface{}) bool {
	c.req <- &req{key: requestKey, data: data}
	return (<-c.res).exists
}

// Delete removes an item and returns true if it existed.
func (c *Cache) Delete(requestKey string) bool {
	c.req <- &req{key: requestKey, del: true}
	ret := <-c.res

	return ret.exists
}
