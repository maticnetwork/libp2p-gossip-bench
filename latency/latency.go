package latency

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultAvgLatency = "0.58"
const jsonFileLocation = "./data.json"
const latenciesUrl = "https://wondernetwork.com/ping-data?sources=%s&destinations=%s"

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

func (l *LatencyData) FindLatency(from, to string) time.Duration {
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

// ReadLatencyData reads data from a downloaded data.json file
// if data.json is not present, it tries to download it
func ReadLatencyData() *LatencyData {
	data, err := os.ReadFile(jsonFileLocation)
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
	url := fmt.Sprintf(latenciesUrl, ids, ids)
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
	err = os.WriteFile(jsonFileLocation, data, 0644)
	if err != nil {
		panic(err)
	}

	return data
}
