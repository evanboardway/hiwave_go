package types

import (
	"sync"

	"github.com/gorilla/websocket"
)

type ThreadSafeWriter struct {
	Conn  *websocket.Conn
	Mutex sync.RWMutex
}

func (t *ThreadSafeWriter) WriteJSON(v interface{}) error {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()

	return t.Conn.WriteJSON(v)
}
