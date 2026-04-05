//nolint:gochecknoglobals
package exp

import (
	"expvar"
	"time"

	"github.com/hako/durafmt"
)

var mainMap = expvar.NewMap("auth").Init()

// @Description  Retrieve internal application statistics.
// @Summary      Return auth proxy stats
// @Tags         stats
// @Produce      json
// @Success      200  {object} any "Auth Proxy Stats"
// @Router       /stats [get]
func init() { //nolint:gochecknoinits
	start := &Time{Time: time.Now(), Places: ThreePlaces}
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

// TimeLength is the length of the time duration to format.
type TimeLength int

// Time allows formatting time Durations.
type Time struct {
	time.Time

	Places TimeLength
}

const (
	// OnePlace is the number of places to format the time duration.
	OnePlace TimeLength = 1
	// TwoPlaces is the number of places to format the time duration.
	TwoPlaces TimeLength = 2
	// ThreePlaces is the number of places to format the time duration.
	ThreePlaces TimeLength = 3
	// DefaultPlaces is the default number of places to format the time duration.
	DefaultPlaces = TwoPlaces
)

// Since returns a pretty-formatted time duration for expvar.
func (e *Time) Since() any {
	num := e.Places
	if num == 0 {
		num = DefaultPlaces
	}

	return durafmt.Parse(time.Since(e.Time)).LimitFirstN(int(num)).String()
}
