package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Hub struct {
	mu    sync.RWMutex
	rooms map[uuid.UUID]map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{rooms: make(map[uuid.UUID]map[*websocket.Conn]struct{})}
}

func (h *Hub) ServeServerSocket(w http.ResponseWriter, r *http.Request, serverUUID uuid.UUID) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	h.subscribe(serverUUID, conn)
	defer h.unsubscribe(serverUUID, conn)

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Hub) Broadcast(serverUUID uuid.UUID, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("ws broadcast marshal failed: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.rooms[serverUUID] {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("ws write failed: %v", err)
		}
	}
}

func (h *Hub) subscribe(serverUUID uuid.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[serverUUID] == nil {
		h.rooms[serverUUID] = make(map[*websocket.Conn]struct{})
	}
	h.rooms[serverUUID][conn] = struct{}{}
}

func (h *Hub) unsubscribe(serverUUID uuid.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[serverUUID], conn)
	if len(h.rooms[serverUUID]) == 0 {
		delete(h.rooms, serverUUID)
	}
}
