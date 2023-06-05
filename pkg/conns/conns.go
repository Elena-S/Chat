package conns

import (
	"errors"
	"sync"
	"sync/atomic"

	"golang.org/x/net/websocket"
)

var Pool *manager = new(manager)

type manager struct {
	pool       sync.Map
	currentNum uint64
	mu         sync.Mutex
}

type connection struct {
	// ws  *websocket.Conn
	num uint64
}

func (m *manager) Store(userID uint, ws *websocket.Conn) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cs, _, err := m.retrieveConns(userID)
	if err != nil {
		return 0, err
	}
	atomic.AddUint64(&m.currentNum, 1)
	conn := connection{num: m.currentNum}
	cs.Store(ws, conn)
	m.pool.Store(userID, cs)
	return conn.num, nil
}

func (m *manager) Get(userID uint) (cs sync.Map, err error) {
	cs, ok, err := m.retrieveConns(userID)
	if err != nil {
		return
	}
	if !ok {
		return cs, errors.New("conns: the contact is offline")
	}
	return cs, nil
}

func (m *manager) CloseAndDelete(userID uint, ws *websocket.Conn) (err error) {
	ws.Close()
	if userID == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	cs, ok, err := m.retrieveConns(userID)
	if err != nil || !ok {
		return err
	}
	cs.Delete(ws)
	m.pool.Store(userID, cs)

	return
}

func (m *manager) retrieveConns(userID uint) (cs sync.Map, ok bool, err error) {
	value, ok := m.pool.Load(userID)
	if ok {
		if cs, ok = value.(sync.Map); !ok {
			return cs, ok, errors.New("conns: got wrong type")
		}
	}
	return cs, ok, nil
}

func (m *manager) UserStatus(userID uint) int8 {
	_, err := m.Get(userID)
	if err != nil {
		return 0
	}
	return 1
}
