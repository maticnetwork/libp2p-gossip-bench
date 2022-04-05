package latency

import (
	_ "embed"
	"encoding/json"
	"math/rand"
	"time"
)

const defaultAvgLatency = "0.58"

//go:embed latencies.json
var latencyDataRaw string

type LatencyData struct {
	PingData map[string]map[string]struct {
		Avg string
	}
	SourcesList []struct {
		Id   string
		Name string
	}
	sources map[string]string
}

func (l *LatencyData) GetRandomCity() string {
	n := rand.Int() % len(l.SourcesList)
	city := l.SourcesList[n].Name
	return city
}

func (l *LatencyData) Find(from, to string) time.Duration {
	fromID := l.sources[from]
	toID := l.sources[to]

	pd := l.PingData[fromID][toID].Avg
	if pd == "" { // there are some cities which are not connected in json file
		pd = defaultAvgLatency
	}

	dur, err := time.ParseDuration(pd + "ms")
	if err != nil {
		panic(err)
	}
	return dur
}

func ReadLatencyDataFromJson() *LatencyData {
	var latencyData LatencyData
	if err := json.Unmarshal([]byte(latencyDataRaw), &latencyData); err != nil {
		panic(err)
	}

	latencyData.sources = map[string]string{}
	for _, i := range latencyData.SourcesList {
		latencyData.sources[i.Name] = i.Id
	}
	return &latencyData
}
