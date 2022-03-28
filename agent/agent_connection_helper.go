package agent

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	ma "github.com/multiformats/go-multiaddr"
)

type agentConnectionHelper struct {
	wgs  map[int64]*sync.WaitGroup
	lock *sync.RWMutex
}

func newAgentConnectionHelper() agentConnectionHelper {
	return agentConnectionHelper{
		wgs:  make(map[int64]*sync.WaitGroup),
		lock: &sync.RWMutex{},
	}
}

func (acg *agentConnectionHelper) Add(laddr, raddr ma.Multiaddr) error {
	key := acg.getKey(laddr, raddr, true)
	acg.lock.RLock()
	wg := acg.wgs[key]
	acg.lock.RUnlock()
	if wg != nil {
		return fmt.Errorf("connection (%s, %s) already exists", laddr, raddr)
	}
	wg = &sync.WaitGroup{}
	wg.Add(2)
	acg.lock.Lock()
	acg.wgs[key] = wg
	acg.lock.Unlock()
	return nil
}

func (acg *agentConnectionHelper) Wait(laddr, raddr ma.Multiaddr) error {
	wg, err := acg.getWaitGroup(laddr, raddr, true)
	if err != nil {
		return err
	}
	wg.Wait()
	return nil
}

func (acg *agentConnectionHelper) WaitWithTimeout(laddr, raddr ma.Multiaddr, timeout time.Duration) error {
	wg, err := acg.getWaitGroup(laddr, raddr, true)
	if err != nil {
		return err
	}
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-c:
	case <-timer.C:
	}
	return nil
}

func (acg *agentConnectionHelper) Delete(laddr, raddr ma.Multiaddr) {
	key := acg.getKey(laddr, raddr, true)
	acg.lock.Lock()
	defer acg.lock.Unlock()
	delete(acg.wgs, key)
}

func (acg *agentConnectionHelper) Connected(laddr, raddr ma.Multiaddr, outbound bool) error {
	wg, err := acg.getWaitGroup(laddr, raddr, outbound)
	if err != nil {
		return err
	}
	wg.Done()
	return nil
}

func (acg *agentConnectionHelper) getWaitGroup(laddr, raddr ma.Multiaddr, outbound bool) (*sync.WaitGroup, error) {
	key := acg.getKey(laddr, raddr, outbound)
	acg.lock.RLock()
	wg := acg.wgs[key]
	acg.lock.RUnlock()
	if wg == nil {
		return nil, fmt.Errorf("connection (%s, %s) does not exist", laddr, raddr)
	}
	return wg, nil
}

func (acg *agentConnectionHelper) getKey(laddr, raddr ma.Multiaddr, outbound bool) int64 {
	lportStr, _ := laddr.ValueForProtocol(ma.P_TCP)
	rportStr, _ := raddr.ValueForProtocol(ma.P_TCP)
	lport, _ := strconv.Atoi(lportStr)
	rport, _ := strconv.Atoi(rportStr)
	if outbound {
		return int64(lport) + (int64(rport) << 32)
	}
	return int64(rport) + (int64(lport) << 32)
}
