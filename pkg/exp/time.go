package exp

import (
	"sync"
	"time"

	"github.com/hako/durafmt"
)

type Time struct {
	time.Time
	Num int
}

type Dur struct {
	time.Duration
	Num int
	mu  sync.RWMutex
}

func (e *Time) Since() interface{} {
	num := e.Num
	if num == 0 {
		num = 2
	}

	return durafmt.Parse(time.Since(e.Time)).LimitFirstN(num).String()
}

func (d *Dur) Add(add time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Duration += add
}

func (d *Dur) Int() interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	num := d.Num
	if num == 0 {
		num = 3
	}

	return durafmt.Parse(d.Duration).LimitFirstN(num).String()
}
