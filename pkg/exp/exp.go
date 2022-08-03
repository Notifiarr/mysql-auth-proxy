//nolint:gochecknoglobals
package exp

import (
	"expvar"
	"time"
)

var (
	mainMap = expvar.NewMap("auth").Init()
)

func init() {
	start := &Time{Time: time.Now(), Num: 3}
	mainMap.Set("Uptime", expvar.Func(start.Since))
}

var (
	// LogFiles      = GetMap("Log File Information").Init()
	HTTPRequests  = GetMap("Incoming HTTP Requests").Init()
	MySQLRequests = GetMap("Outgoing MySQL Requests").Init()
)

func GetMap(name string) *expvar.Map {
	if p := mainMap.Get(name); p != nil {
		pp, _ := p.(*expvar.Map)
		return pp
	}

	newMap := expvar.NewMap(name)
	mainMap.Set(name, newMap)

	return newMap
}

func GetKeys(mapName *expvar.Map) map[string]interface{} {
	output := make(map[string]interface{})

	mapName.Do(func(keyval expvar.KeyValue) {
		switch v := keyval.Value.(type) {
		case *expvar.Int:
			output[keyval.Key] = v.Value()
		case expvar.Func:
			output[keyval.Key], _ = v.Value().(int64)
		default:
			output[keyval.Key] = keyval.Value
		}
	})

	return output
}
