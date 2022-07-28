package lock

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
)

var (
	manager   *lockManager
	nextToken uint64 = 0
)

type LockType uint8

const (
	TypeRead LockType = iota + 1
	TypeWrite
)

type lockState uint8

const (
	StateInit lockState = iota
	stateHolding
	stateWaiting
)

type requestID uint64

type lockInfo struct {
	ResourceID uint64
	typ        LockType
	start      uint64
	end        uint64
}

type Lock struct {
	info *lockInfo

	mu sync.Mutex
	// All request on this lock. unlock should always release the lock from requestQueue.Front()
	requestQueue *list.List
}

type lockRequest struct {
	*lockInfo
	// id for each lock request
	token requestID

	state lockState
	mu    sync.Mutex
	cond  *sync.Cond
}

type lockManager struct {
	mu sync.RWMutex
	// key: resourceID
	// value: all lock request on the resourceID
	lockSets map[uint64]*lockSet
}

type lockSet struct {
	mu sync.Mutex
	// All lock request on the same resourceId
	lockRequests map[requestID]*list.Element

	// granted lock request
	holders *list.List
	// ungranted lock request
	waiters *list.List
}

func (m *lockManager) getLockSet(id uint64) *lockSet {
	m.mu.RLock()
	if lks, found := m.lockSets[id]; found {
		m.mu.RUnlock()
		return lks
	}

	m.mu.RUnlock()
	m.mu.Lock()
	if lks, found := m.lockSets[id]; found {
		m.mu.Unlock()
		return lks
	}

	lks := &lockSet{
		lockRequests: make(map[requestID]*list.Element),
		holders:      list.New(),
		waiters:      list.New(),
	}
	m.lockSets[id] = lks
	m.mu.Unlock()
	return lks
}

func New(resourceID uint64, typ LockType, start, end uint64) *Lock {
	if start >= end {
		panic("invalid lock range")
	}

	if typ != TypeRead && typ != TypeWrite {
		panic("invalid lock type")
	}

	lk := &Lock{
		info: &lockInfo{
			ResourceID: resourceID,
			typ:        typ,
			start:      start,
			end:        end,
		},
		requestQueue: list.New(),
	}
	return lk
}

func (lk *Lock) Lock() {
	lks := manager.getLockSet(lk.info.ResourceID)

	req := &lockRequest{
		token:    requestID(atomic.AddUint64(&nextToken, 1)),
		lockInfo: lk.info,
		state:    StateInit,
	}
	req.cond = sync.NewCond(&req.mu)

	lk.mu.Lock()
	lk.requestQueue.PushBack(req)
	lk.mu.Unlock()

	lks.aquire(req)
}

func (lk *Lock) Unlock() {
	manager.mu.RLock()
	lks, found := manager.lockSets[lk.info.ResourceID]
	if !found {
		panic("invalid lock")
	}
	manager.mu.RUnlock()

	lk.mu.Lock()
	// May exist more than one request, must unlock the 1st request
	v := lk.requestQueue.Remove(lk.requestQueue.Front())
	if v == nil {
		panic("unlock an non-granted lock")
	}
	lk.mu.Unlock()

	req := v.(*lockRequest)
	req.mu.Lock()
	if req.state != stateHolding {
		panic("unlock an non-granted lock")
	}
	req.mu.Unlock()

	lks.Release(req)
}

func (lks *lockSet) aquire(req *lockRequest) {
	lks.mu.Lock()

	conflict := false
	for i := lks.waiters.Front(); i != nil; i = i.Next() {
		v := i.Value
		w := v.(*lockRequest)
		if req.start >= w.end || req.end <= w.start {
			continue
		}
		conflict = true
		break
	}

	if !conflict {
		for i := lks.holders.Front(); i != nil; i = i.Next() {
			v := i.Value
			h := v.(*lockRequest)
			if req.start >= h.end || req.end <= h.start {
				continue
			}
			conflict = true
			break
		}
	}

	if !conflict {
		lks.lockRequests[req.token] = lks.holders.PushBack(req)

		req.mu.Lock()
		req.state = stateHolding
		req.mu.Unlock()

		lks.mu.Unlock()
		return
	}

	lks.lockRequests[req.token] = lks.waiters.PushBack(req)
	lks.mu.Unlock()

	req.mu.Lock()
	req.state = stateWaiting
	for req.state != stateHolding {
		req.cond.Wait()
	}
	req.mu.Unlock()
}

func (lks *lockSet) Release(req *lockRequest) {
	lks.mu.Lock()
	lks.holders.Remove(lks.lockRequests[req.token])
	delete(lks.lockRequests, req.token)

	for i := lks.waiters.Front(); i != nil; {
		next := i.Next()

		v := i.Value
		w := v.(*lockRequest)
		conflict := false
		for j := lks.holders.Front(); j != nil; j = j.Next() {
			v := j.Value
			h := v.(*lockRequest)
			if w.typ == TypeRead && h.typ == TypeRead || w.start >= h.end || w.end <= h.start {
				continue
			}
			conflict = true
			break
		}

		if !conflict {
			// 1. Remove the waiter from waiter
			// 2. Add it into holder
			// 3. Change lockRequest state
			// 4. Signal cond to return
			lks.waiters.Remove(i)
			lks.lockRequests[w.token] = lks.holders.PushBack(w)

			w.mu.Lock()
			w.state = stateHolding
			w.cond.Signal()
			w.mu.Unlock()
		}
		i = next
	}
	lks.mu.Unlock()
}

func (m *lockManager) cleanStaleLocks() {
	ticker := time.NewTicker(time.Second * 1)
	for {
		<-ticker.C
		m.mu.Lock()
		for k, v := range m.lockSets {
			v.mu.Lock()
			if len(v.lockRequests) == 0 {
				delete(m.lockSets, k)
			}
			v.mu.Unlock()
		}
		m.mu.Unlock()
	}
}

func init() {
	manager = &lockManager{
		lockSets: make(map[uint64]*lockSet),
	}
	go manager.cleanStaleLocks()
}
