package latency

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

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

func (l *LatencyData) FindLatency(from, to string) time.Duration {
	fromID := l.sources[from]
	toID := l.sources[to]

	dur, err := time.ParseDuration(l.PingData[fromID][toID].Avg + "ms")
	if err != nil {
		panic(err)
	}
	return dur
}

// ReadLatencyData reads data from a downloaded data.json file
// if data.json is not present, it tries to download it
func ReadLatencyData() *LatencyData {
	data, err := ioutil.ReadFile("./data.json")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			data = getLatencyData()
		} else {
			panic(err)
		}
	}

	var latencyData LatencyData
	if err := json.Unmarshal(data, &latencyData); err != nil {
		panic(err)
	}

	latencyData.sources = map[string]string{}
	for _, i := range latencyData.SourcesList {
		latencyData.sources[i.Name] = i.Id
	}
	return &latencyData
}

func getLatencyData() []byte {
	idsSlice := []string{}

	for i := 0; i < 50; i++ {
		idsSlice = append(idsSlice, strconv.Itoa(i))
	}

	ids := strings.Join(idsSlice, ",")
	url := "https://wondernetwork.com/ping-data?sources=" + ids + "&destinations=" + ids

	req, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer req.Body.Close()

	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		panic(err)
	}

	// write data in a file, for later
	err = os.WriteFile("./data.json", data, 0644)
	if err != nil {
		panic(err)
	}

	return data
}
