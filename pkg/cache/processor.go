package cache

import (
	"math/rand"
	"time"
)

const (
	maxUnusedAge  = 25 * time.Hour
	pruneInterval = 4 * time.Minute // some seconds are added to this.
	expireAfter   = 15 * time.Minute
	prune         = "prune"
	getList       = "getList"
)

// req is our request (input channel).
type req struct {
	key    string
	del    bool
	data   interface{}
	expire bool
}

// res is our response (output channel).
type res struct {
	item   *Item
	exists bool
	len    int
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

func (c *Cache) pruneRequests() *time.Ticker {
	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1).Intn(30)
	ticker := time.NewTicker(pruneInterval + time.Duration(r1)*time.Second)

	go func() {
		for range ticker.C {
			c.req <- &req{del: true, key: prune}
			res := <-c.res

			c.pruneTime.Add(time.Since(res.item.Time))
			c.exp.Add("Pruned", res.item.Hits)
			c.len.Set(int64(res.len))
			c.exp.Add("Prune Runs", 1)
		}
	}()

	return ticker
}

// processRequests is the main running go routine for the cache.
func (c *Cache) processRequests() {
	defer close(c.res) // close response channel when request channel closes.

	if c.prune {
		ticker := c.pruneRequests()
		defer ticker.Stop()
	}

	for req := range c.req {
		switch {
		case req.key == getList:
			c.res <- c.list()
		case req.del && req.key == prune:
			c.res <- c.pruneItems() // special
		case req.data != nil:
			c.res <- c.save(req)
		case req.del:
			c.res <- c.delete(req.key)
		default:
			c.res <- c.get(req.key)
		}
	}
}

func (c *Cache) save(req *req) *res {
	_, exists := c.cache[req.key]
	now := time.Now()
	c.cache[req.key] = &Item{Data: req.data, Time: now, expire: req.expire, Last: now}

	return &res{item: c.cache[req.key], exists: exists, len: len(c.cache)}
}

func (c *Cache) get(key string) *res {
	item, exists := c.cache[key]
	if exists {
		item = item.copy() // make a copy
	}

	return &res{item: item, exists: exists, len: len(c.cache)}
}

func (item *Item) copy() *Item {
	item.Hits++
	item.Last = time.Now()

	return &Item{
		Data: item.Data,
		Time: item.Time,
		Last: item.Last,
		Hits: item.Hits,
	}
}

func (c *Cache) list() *res {
	list := make(map[string]*Item)
	for key, item := range c.cache {
		list[key] = item.copy()
	}

	return &res{item: &Item{Data: list}}
}

func (c *Cache) delete(key string) *res {
	_, exists := c.cache[key]
	c.cache[key] = nil
	delete(c.cache, key)

	return &res{exists: exists, len: len(c.cache)}
}

func (c *Cache) pruneItems() *res {
	pruned := 0
	start := time.Now()

	for key, item := range c.cache {
		last := time.Since(item.Last)
		if last > maxUnusedAge || (item.expire && last > expireAfter) {
			c.cache[key] = nil
			delete(c.cache, key)
			pruned++
		}
	}

	return &res{len: len(c.cache), item: &Item{Time: start, Hits: int64(pruned)}}
}
