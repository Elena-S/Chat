package conns

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/Elena-S/Chat/pkg/users"
	"go.uber.org/fx"
	"golang.org/x/net/websocket"
)

var (
	ErrConnNotRegister    = errors.New("conns: a websocket connection did not register")
	ErrInvalidPayloadType = errors.New("conns: invalid payload type")
	ErrInvalidPoolType    = errors.New("conns: invalid pool type")
	ErrContactOffline     = errors.New("conns: the contact is offline")
)

var Module = fx.Module("conns",
	fx.Provide(
		NewManager,
	),
)

type Manager struct {
	pool       *sync.Map
	currentNum uint
	mu         sync.Mutex
}

type ManagerParams struct {
	fx.In
}

func NewManager(p ManagerParams) *Manager {
	return &Manager{pool: new(sync.Map)}
}

type payload struct {
	num uint
}

func (m *Manager) Store(userID users.UserID, ws *websocket.Conn) (uint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cs, _, err := m.retrieveConns(userID)
	if err != nil {
		return 0, err
	}

	m.currentNum++
	conn := payload{num: m.currentNum}

	cs.Store(ws, conn)
	m.pool.Store(userID, cs)

	return conn.num, nil
}

func (m *Manager) Get(userID users.UserID) (cs *sync.Map, err error) {
	cs, ok, err := m.retrieveConns(userID)
	if err != nil {
		return
	}
	if !ok {
		return cs, ErrContactOffline
	}
	return cs, nil
}

func (m *Manager) CloseAndDelete(userID users.UserID, ws *websocket.Conn) (err error) {
	err = ws.Close()
	if !errors.Is(err, net.ErrClosed) {
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
		err = ErrConnNotRegister
		return
	}
	conn, ok := data.(payload)
	if !ok {
		err = fmt.Errorf("%w: payload data does not match type %T, got type %T", ErrInvalidPayloadType, conn, data)
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
	return nil
}

func (m *Manager) retrieveConns(userID users.UserID) (cs *sync.Map, ok bool, err error) {
	value, ok := m.pool.Load(userID)
	if ok {
		if cs, ok = value.(*sync.Map); !ok {
			return cs, ok, fmt.Errorf("%w: loaded data does not match type %T, got type %T", ErrInvalidPoolType, cs, value)
		}
	} else {
		cs = new(sync.Map)
	}
	return cs, ok, nil
}

func (m *Manager) UserStatus(userID users.UserID) int8 {
	_, err := m.Get(userID)
	if err != nil {
		return 0
	}
	return 1
}
