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
	conn.SetReadLimit(32 * 1024)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Logs come first and unconditionally: ContainerLogs works against a
	// container's stored output regardless of its current state, unlike
	// Attach, which needs it running *right now*. A crash-looping container
	// (bad startup command, image pull failure, whatever) is essentially
	// never in a stable running state long enough to attach to — that's
	// exactly when seeing *why* it's crashing matters most, so failing the
	// whole connection on Attach alone used to hide the one thing that
	// would explain the problem.
	logs, err := h.docker.LogsFollow(ctx, containerID)
	if err != nil {
		log.Printf("logs follow failed: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("[console] could not read container logs: "+err.Error()))
		return
	}
	defer logs.Close()
	go h.pumpLogs(conn, logs)

	stdin, err := h.docker.Attach(ctx, containerID)
	if err != nil {
		log.Printf("attach stdin failed: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("[console] read-only — could not attach for input (is the server running?): "+err.Error()))
		stdin = nil
	} else {
		h.setWriter(serverUUID, stdin)
		defer h.clearWriter(serverUUID)
		defer stdin.Close()
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if stdin == nil {
			conn.WriteMessage(websocket.TextMessage, []byte("[console] can't send input — not attached to a running container"))
			continue
		}
		if _, err := stdin.Write(append(msg, '\n')); err != nil {
			log.Printf("write stdin failed: %v", err)
			conn.WriteMessage(websocket.TextMessage, []byte("[console] failed to send command: "+err.Error()))
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

func (h *Hub) SendCommand(ctx context.Context, serverUUID uuid.UUID, containerID, command string) error {
	h.mu.Lock()
	w, ok := h.writers[serverUUID]
	h.mu.Unlock()
	if ok {
		_, err := w.Write([]byte(command + "\n"))
		return err
	}

	stdin, err := h.docker.Attach(ctx, containerID)
	if err != nil {
		return err
	}
	defer stdin.Close()
	_, err = stdin.Write([]byte(command + "\n"))
	return err
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
