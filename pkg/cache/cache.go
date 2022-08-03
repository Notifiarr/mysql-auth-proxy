package cache

import (
	"expvar"
	"time"

	"github.com/Notifiarr/mysql-auth-proxy/pkg/exp"
)

// Cache provides methods to get a user and delete a user from cache.
// If the user is not in cache it is fetched using the userinfo module.
type Cache struct {
	cache     map[string]*Item
	req       chan *req
	res       chan *res
	exp       *expvar.Map
	len       expvar.Int
	prune     bool
	pruneTime exp.Dur
}

// Item is what's returned from a cache Get.
type Item struct {
	Data   interface{} `json:"data"`
	Time   time.Time   `json:"created"`
	Last   time.Time   `json:"last_access"`
	Hits   int64       `json:"hits"`
	expire bool
}

// New starts the cache routine and returns a struct to get data from the cache.
func New(name string, prune bool) *Cache {
	cache := &Cache{exp: exp.GetMap(name + " Cache"), prune: prune}
	cache.exp.Set("Size", &cache.len)
	cache.start()

	if prune {
		cache.exp.Set("Prune Time", expvar.Func(cache.pruneTime.Int))
	}

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

	c.len.Set(int64(ret.len))
	c.exp.Add("Get", 1)

	if ret.exists {
		c.exp.Add("Hit", 1)
	} else {
		c.exp.Add("Miss", 1)
	}

	return ret.item
}

// Save saves an item, and returns true if it already existed.
func (c *Cache) Save(requestKey string, data interface{}, expire bool) bool {
	c.req <- &req{key: requestKey, data: data, expire: expire}
	ret := <-c.res

	c.len.Set(int64(ret.len))

	if ret.exists {
		c.exp.Add("Update", 1)
	} else {
		c.exp.Add("Save", 1)
	}

	return ret.exists
}

// Delete removes an item and returns true if it existed.
func (c *Cache) Delete(requestKey string) bool {
	c.req <- &req{key: requestKey, del: true}
	ret := <-c.res

	c.len.Set(int64(ret.len))
	c.exp.Add("Delete", 1)

	return ret.exists
}

func (c *Cache) List() map[string]*Item {
	c.req <- &req{key: getList}
	ret := <-c.res

	return ret.item.Data.(map[string]*Item)
}
