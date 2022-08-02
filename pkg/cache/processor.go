package cache

import "time"

// req is our request (input channel).
type req struct {
	key  string
	del  bool
	data interface{}
}

// res is our response (output channel).
type res struct {
	item   *Item
	exists bool
}

func (c *Cache) start() {
	c.cache = make(map[string]*Item)
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
	for k := range c.cache {
		c.cache[k] = nil
		delete(c.cache, k)
	}

	c.cache = nil
}

// processRequests is the main running go routine for the cache.
func (c *Cache) processRequests() {
	defer close(c.res) // close response channel when request channel closes.

	for req := range c.req {
		switch {
		case req.data != nil:
			_, exists := c.cache[req.key]
			c.cache[req.key] = &Item{Data: req.data, Time: time.Now()}
			c.res <- &res{item: c.cache[req.key], exists: exists}
		case req.del:
			_, exists := c.cache[req.key]
			c.cache[req.key] = nil
			delete(c.cache, req.key)
			c.res <- &res{exists: exists}
		default:
			data, exists := c.cache[req.key]
			c.res <- &res{item: data, exists: exists}
		}
	}
}
