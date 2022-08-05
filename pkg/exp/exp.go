//nolint:gochecknoglobals
package exp

import (
	"expvar"
	"time"

	"github.com/hako/durafmt"
)

var mainMap = expvar.NewMap("auth").Init()

func init() {
	start := &Time{Time: time.Now(), Num: 3}
	mainMap.Set("Uptime", expvar.Func(start.Since))
}

// GetMap returns a known-expvar map by name.
func GetMap(name string) *expvar.Map {
	if p := mainMap.Get(name); p != nil {
		pp, _ := p.(*expvar.Map)
		return pp
	}

	newMap := expvar.NewMap(name)
	mainMap.Set(name, newMap)

	return newMap
}

// AddVar allows adding arbitrary vars or maps to our main map.
func AddVar(name string, newVar expvar.Var) {
	mainMap.Set(name, newVar)
}

// Time allows formatting time Durations.
type Time struct {
	time.Time
	Num int
}

// Since returns a pretty-formatted time duration for expvar.
func (e *Time) Since() interface{} {
	num := e.Num
	if num == 0 {
		num = 2
	}

	return durafmt.Parse(time.Since(e.Time)).LimitFirstN(num).String()
}
