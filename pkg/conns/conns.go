package conns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/Elena-S/Chat/pkg/broker"
	"github.com/Elena-S/Chat/pkg/users"
	"golang.org/x/net/websocket"
)

var Pool *manager = &manager{pool: new(sync.Map)}

type manager struct {
	pool       *sync.Map
	currentNum uint
	mu         sync.Mutex
}

type payload struct {
	num    uint
	values map[any]any
}

func (m *manager) Store(userID uint, ws *websocket.Conn) (uint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cs, _, err := m.retrieveConns(userID)
	if err != nil {
		return 0, err
	}

	values := map[any]any{}
	err = broker.Subscribe(context.TODO(), users.IDToString(userID), ws, values)
	if err != nil {
		return 0, err
	}

	m.currentNum++
	conn := payload{num: m.currentNum, values: values}

	cs.Store(ws, conn)
	m.pool.Store(userID, cs)

	return conn.num, nil
}

func (m *manager) Get(userID uint) (cs *sync.Map, err error) {
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
	err = ws.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return
	}
	if userID == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	cs, ok, err := m.retrieveConns(userID)
	if err != nil || !ok {
		return
	}
	data, ok := cs.LoadAndDelete(ws)
	if !ok {
		err = errors.New("conns: a websocket connection did not register")
		return
	}
	conn, ok := data.(payload)
	if !ok {
		err = fmt.Errorf("conns: payload data does not match type %T, got type %T", conn, data)
		return
	}
	empty := true
	cs.Range(func(key any, value any) bool {
		empty = false
		return false
	})
	if !empty {
		m.pool.Store(userID, cs)
		return
	}
	m.pool.Delete(userID)
	return broker.Unsubscribe(context.TODO(), users.IDToString(userID), conn.values)
}

func (m *manager) retrieveConns(userID uint) (cs *sync.Map, ok bool, err error) {
	value, ok := m.pool.Load(userID)
	if ok {
		if cs, ok = value.(*sync.Map); !ok {
			return cs, ok, fmt.Errorf("conns: loaded data does not match type %T, got type %T", cs, value)
		}
	} else {
		cs = new(sync.Map)
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
