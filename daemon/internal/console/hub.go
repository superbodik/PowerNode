package console

import (
	"bufio"
	"context"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/yourorg/panel-daemon/internal/docker"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Hub struct {
	docker *docker.Manager

	mu      sync.Mutex
	writers map[uuid.UUID]io.WriteCloser
}

func NewHub(dockerManager *docker.Manager) *Hub {
	return &Hub{docker: dockerManager, writers: make(map[uuid.UUID]io.WriteCloser)}
}

func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, serverUUID uuid.UUID, containerID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("console ws upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	stdin, err := h.docker.Attach(ctx, containerID)
	if err != nil {
		log.Printf("attach stdin failed: %v", err)
		return
	}
	h.setWriter(serverUUID, stdin)
	defer h.clearWriter(serverUUID)
	defer stdin.Close()

	logs, err := h.docker.LogsFollow(ctx, containerID)
	if err != nil {
		log.Printf("logs follow failed: %v", err)
		return
	}
	defer logs.Close()

	go h.pumpLogs(conn, logs)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if _, err := stdin.Write(append(msg, '\n')); err != nil {
			log.Printf("write stdin failed: %v", err)
			return
		}
	}
}

func (h *Hub) pumpLogs(conn *websocket.Conn, logs io.Reader) {
	scanner := bufio.NewScanner(logs)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := conn.WriteMessage(websocket.TextMessage, scanner.Bytes()); err != nil {
			return
		}
	}
}

func (h *Hub) setWriter(serverUUID uuid.UUID, w io.WriteCloser) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writers[serverUUID] = w
}

func (h *Hub) clearWriter(serverUUID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.writers, serverUUID)
}
