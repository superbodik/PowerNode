package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
)

type Manager struct {
	cli     *client.Client
	dataDir string
}

func NewManager(dockerHost, dataDir string) (*Manager, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if dockerHost != "" {
		opts = append(opts, client.WithHost(dockerHost))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Manager{cli: cli, dataDir: dataDir}, nil
}

type CreateSpec struct {
	ServerUUID     uuid.UUID
	DockerImage    string
	StartupCommand string
	Environment    map[string]string

	MemoryMB   int64
	SwapMB     int64
	IOWeight   int
	CPUPercent int

	PortBindings map[string]string
}

func (m *Manager) serverVolumePath(serverUUID uuid.UUID) string {
	return filepath.Join(m.dataDir, serverUUID.String())
}

func (m *Manager) ServerVolumePath(serverUUID uuid.UUID) string {
	return m.serverVolumePath(serverUUID)
}

func (m *Manager) CreateContainer(ctx context.Context, spec CreateSpec) (string, error) {
	if err := m.ensureImage(ctx, spec.DockerImage); err != nil {
		return "", err
	}

	if err := os.MkdirAll(m.serverVolumePath(spec.ServerUUID), 0755); err != nil {
		return "", fmt.Errorf("create server data directory: %w", err)
	}

	env := make([]string, 0, len(spec.Environment))
	for k, v := range spec.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	memBytes := spec.MemoryMB * 1024 * 1024
	swapBytes := memBytes + spec.SwapMB*1024*1024

	resources := container.Resources{
		Memory:      memBytes,
		MemorySwap:  swapBytes,
		BlkioWeight: uint16(spec.IOWeight),
	}
	if spec.CPUPercent > 0 {
		resources.CPUPeriod = 100000
		resources.CPUQuota = int64(spec.CPUPercent) * 1000
	}

	portBindings, exposedPorts, err := toPortMap(spec.PortBindings)
	if err != nil {
		return "", fmt.Errorf("invalid port bindings: %w", err)
	}
	openFirewallPorts(spec.PortBindings)

	var entrypoint, cmd []string
	if spec.StartupCommand != "" {
		entrypoint = []string{"/bin/sh", "-c"}
		cmd = []string{spec.StartupCommand}
	}

	containerName := "srv-" + spec.ServerUUID.String()
	created, err := m.cli.ContainerCreate(ctx, &container.Config{
		Image:        spec.DockerImage,
		Entrypoint:   entrypoint,
		Cmd:          cmd,
		Env:          env,
		ExposedPorts: exposedPorts,
		Tty:          true,
		OpenStdin:    true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   "/home/container",
	}, &container.HostConfig{
		Resources:    resources,
		PortBindings: portBindings,
		Mounts: []mount.Mount{{
			Type:   mount.TypeBind,
			Source: m.serverVolumePath(spec.ServerUUID),
			Target: "/home/container",
		}},
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
	}, nil, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	return created.ID, nil
}

func (m *Manager) RecreateContainer(ctx context.Context, spec CreateSpec) error {
	name := ContainerNameFor(spec.ServerUUID)
	oldName := name + "-rebuild"

	wasRunning := false
	if state, err := m.InspectState(ctx, name); err == nil {
		wasRunning = state == "running"
	}

	_ = m.cli.ContainerRemove(ctx, oldName, container.RemoveOptions{Force: true, RemoveVolumes: true})

	hadContainer := true
	if err := m.cli.ContainerRename(ctx, name, oldName); err != nil {
		if !client.IsErrNotFound(err) {
			return fmt.Errorf("rename existing container: %w", err)
		}
		hadContainer = false
	}

	if hadContainer && wasRunning {
		timeout := 30
		if err := m.cli.ContainerStop(ctx, oldName, container.StopOptions{Timeout: &timeout}); err != nil {
			m.restoreContainer(ctx, oldName, name, wasRunning)
			return fmt.Errorf("stop existing container: %w", err)
		}
	}

	if _, err := m.CreateContainer(ctx, spec); err != nil {
		m.restoreContainer(ctx, oldName, name, wasRunning)
		return err
	}

	if hadContainer {
		if err := m.cli.ContainerRemove(ctx, oldName, container.RemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
			return fmt.Errorf("remove previous container: %w", err)
		}
	}

	if wasRunning {
		if err := m.StartContainer(ctx, name); err != nil {
			return fmt.Errorf("start recreated container: %w", err)
		}
	}
	return nil
}

func (m *Manager) restoreContainer(ctx context.Context, oldName, name string, wasRunning bool) {
	if err := m.cli.ContainerRename(ctx, oldName, name); err != nil {
		return
	}
	if wasRunning {
		_ = m.StartContainer(ctx, name)
	}
}

func (m *Manager) ensureImage(ctx context.Context, image string) error {
	_, _, err := m.cli.ImageInspectWithRaw(ctx, image)
	if err == nil {
		return nil
	}
	reader, err := m.cli.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", image, err)
	}
	defer reader.Close()
	_, err = io.Copy(io.Discard, reader)
	return err
}

func (m *Manager) StartContainer(ctx context.Context, containerID string) error {
	if state, err := m.InspectState(ctx, containerID); err == nil && state == "running" {
		return nil
	}
	err := m.cli.ContainerStart(ctx, containerID, container.StartOptions{})
	if err != nil && errdefs.IsNotModified(err) {
		return nil
	}
	return err
}

func (m *Manager) StopContainer(ctx context.Context, containerID string, timeoutSeconds int) error {
	if state, err := m.InspectState(ctx, containerID); err == nil && state == "offline" {
		return nil
	}
	timeout := timeoutSeconds
	err := m.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	if err != nil && errdefs.IsNotModified(err) {
		return nil
	}
	return err
}

func (m *Manager) KillContainer(ctx context.Context, containerID string) error {
	if state, err := m.InspectState(ctx, containerID); err == nil && state == "offline" {
		return nil
	}
	err := m.cli.ContainerKill(ctx, containerID, "SIGKILL")
	if err != nil && (errdefs.IsConflict(err) || errdefs.IsNotModified(err)) {
		return nil
	}
	return err
}

func (m *Manager) RemoveContainer(ctx context.Context, containerID string) error {
	if info, err := m.cli.ContainerInspect(ctx, containerID); err == nil && info.HostConfig != nil {
		closeFirewallPorts(info.HostConfig.PortBindings)
	}
	err := m.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true, RemoveVolumes: true})
	if err != nil && client.IsErrNotFound(err) {
		return nil
	}
	return err
}

func ContainerNameFor(serverUUID uuid.UUID) string {
	return "srv-" + serverUUID.String()
}

func (m *Manager) InspectState(ctx context.Context, containerID string) (string, error) {
	info, err := m.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}
	switch {
	case info.State.Running:
		return "running", nil
	case info.State.Restarting:
		return "starting", nil
	default:
		return "offline", nil
	}
}

func (m *Manager) Stats(ctx context.Context, containerID string) (types.StatsJSON, error) {
	resp, err := m.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return types.StatsJSON{}, err
	}
	defer resp.Body.Close()

	var stats types.StatsJSON
	if err := decodeJSON(resp.Body, &stats); err != nil {
		return types.StatsJSON{}, err
	}
	return stats, nil
}

func (m *Manager) LogsFollow(ctx context.Context, containerID string) (io.ReadCloser, error) {
	return m.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "200",
	})
}

func (m *Manager) Attach(ctx context.Context, containerID string) (io.WriteCloser, error) {
	resp, err := m.cli.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return nil, err
	}
	return resp.Conn, nil
}

func toPortMap(bindings map[string]string) (nat.PortMap, nat.PortSet, error) {
	portMap := make(nat.PortMap, len(bindings))
	portSet := make(nat.PortSet, len(bindings))
	for containerPort, hostPort := range bindings {
		p, err := nat.NewPort(portProto(containerPort), portNumber(containerPort))
		if err != nil {
			return nil, nil, err
		}
		portMap[p] = []nat.PortBinding{{HostPort: hostPort}}
		portSet[p] = struct{}{}
	}
	return portMap, portSet, nil
}

func openFirewallPorts(bindings map[string]string) {
	for containerPort, hostPort := range bindings {
		firewallAllow(portProto(containerPort), hostPort)
	}
}

func closeFirewallPorts(bindings nat.PortMap) {
	for port, hostBindings := range bindings {
		for _, hb := range hostBindings {
			firewallRevoke(port.Proto(), hb.HostPort)
		}
	}
}

func firewallAllow(proto, hostPort string) {
	if proto != "tcp" && proto != "udp" || hostPort == "" {
		return
	}
	if out, err := exec.Command("ufw", "allow", hostPort+"/"+proto, "comment", "server allocation").CombinedOutput(); err != nil {
		log.Printf("firewall: failed to allow %s/%s (continuing without it): %v: %s", hostPort, proto, err, strings.TrimSpace(string(out)))
	}
}

func firewallRevoke(proto, hostPort string) {
	if proto != "tcp" && proto != "udp" || hostPort == "" {
		return
	}
	if out, err := exec.Command("ufw", "delete", "allow", hostPort+"/"+proto).CombinedOutput(); err != nil {
		log.Printf("firewall: failed to revoke %s/%s (continuing without it): %v: %s", hostPort, proto, err, strings.TrimSpace(string(out)))
	}
}

func portProto(spec string) string {
	if _, proto, ok := strings.Cut(spec, "/"); ok {
		return proto
	}
	return "tcp"
}

func portNumber(spec string) string {
	if port, _, ok := strings.Cut(spec, "/"); ok {
		return port
	}
	return spec
}

func decodeJSON(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}
