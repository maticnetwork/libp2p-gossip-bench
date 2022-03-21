package latency

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
	"github.com/maticnetwork/libp2p-gossip-bench/proto"
)

type server struct {
	config *network.Config

	*network.Server

	topic *network.Topic

	topicsLock sync.Mutex
	topics     map[string]struct{}

	City string
}

func newServer(logger hclog.Logger, config *network.Config, city string) *server {
	srv, err := network.NewServer(logger, config)
	if err != nil {
		panic(err)
	}

	// create a gossip protocol
	topic, err := srv.NewTopic("/proto1", &proto.Txn{})
	if err != nil {
		panic(err)
	}

	s := &server{
		config: config,
		Server: srv,
		topic:  topic,
		City:   city,
		topics: map[string]struct{}{},
	}
	topic.Subscribe(func(obj interface{}) {
		s.topicsLock.Lock()
		defer s.topicsLock.Unlock()

		msg := obj.(*proto.Txn)
		hash := hashit(msg.Raw.Value)
		s.topics[hash] = struct{}{}
	})
	return s
}

func (s *server) NumTopics() int {
	s.topicsLock.Lock()
	defer s.topicsLock.Unlock()

	return len(s.topics)
}

func hashit(b []byte) string {
	h := sha256.New()
	h.Write(b)
	dst := h.Sum(nil)
	return hex.EncodeToString(dst)
}
