package sftpd

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/yourorg/panel-daemon/internal/docker"
)

type Server struct {
	docker      *docker.Manager
	panelURL    string
	daemonToken string
	http        *http.Client
	config      *ssh.ServerConfig
}

func NewServer(dockerManager *docker.Manager, panelURL, daemonToken, hostKeyPath string) (*Server, error) {
	s := &Server{
		docker:      dockerManager,
		panelURL:    strings.TrimSuffix(panelURL, "/"),
		daemonToken: daemonToken,
		http:        &http.Client{Timeout: 10 * time.Second},
	}

	signer, err := loadOrCreateHostKey(hostKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load host key: %w", err)
	}

	s.config = &ssh.ServerConfig{PublicKeyCallback: s.authenticate}
	s.config.AddHostKey(signer)
	return s, nil
}

func loadOrCreateHostKey(path string) (ssh.Signer, error) {
	if data, err := os.ReadFile(path); err == nil {
		return ssh.ParsePrivateKey(data)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(priv)
}

type authResponse struct {
	Allowed    bool   `json:"allowed"`
	ReadOnly   bool   `json:"read_only"`
	ServerUUID string `json:"server_uuid"`
	Reason     string `json:"reason"`
}

func (s *Server) authenticate(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	if s.panelURL == "" {
		return nil, fmt.Errorf("SFTP is not configured on this node (no panel URL)")
	}

	body, _ := json.Marshal(map[string]string{
		"username":    conn.User(),
		"fingerprint": ssh.FingerprintSHA256(key),
	})
	req, err := http.NewRequest(http.MethodPost, s.panelURL+"/api/v1/internal/sftp/authenticate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.daemonToken)

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("panel unreachable: %w", err)
	}
	defer resp.Body.Close()

	var result authResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("invalid panel response: %w", err)
	}
	if !result.Allowed {
		if result.Reason == "" {
			result.Reason = "access denied"
		}
		return nil, fmt.Errorf("%s", result.Reason)
	}

	perms := &ssh.Permissions{Extensions: map[string]string{"server_uuid": result.ServerUUID}}
	if result.ReadOnly {
		perms.Extensions["read_only"] = "1"
	}
	return perms, nil
}

func (s *Server) ListenAndServe(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("sftp: listening on %s", addr)

	for {
		nConn, err := listener.Accept()
		if err != nil {
			log.Printf("sftp: accept failed: %v", err)
			continue
		}
		go s.handleConn(nConn)
	}
}

func (s *Server) handleConn(nConn net.Conn) {
	defer nConn.Close()

	sshConn, chans, reqs, err := ssh.NewServerConn(nConn, s.config)
	if err != nil {
		log.Printf("sftp: auth failed for %s: %v", nConn.RemoteAddr(), err)
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	serverUUID, err := uuid.Parse(sshConn.Permissions.Extensions["server_uuid"])
	if err != nil {
		return
	}
	readOnly := sshConn.Permissions.Extensions["read_only"] == "1"
	baseDir := s.docker.ServerVolumePath(serverUUID)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go handleSession(channel, requests, baseDir, readOnly)
	}
}

func handleSession(channel ssh.Channel, requests <-chan *ssh.Request, baseDir string, readOnly bool) {
	defer channel.Close()

	for req := range requests {
		if req.Type != "subsystem" || string(req.Payload[4:]) != "sftp" {
			if req.WantReply {
				req.Reply(false, nil)
			}
			continue
		}
		req.Reply(true, nil)

		h := &fsHandler{baseDir: baseDir, readOnly: readOnly}
		handlers := sftp.Handlers{FileGet: h, FilePut: h, FileCmd: h, FileList: h}
		server := sftp.NewRequestServer(channel, handlers)
		server.Serve()
		return
	}
}
