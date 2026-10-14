package test

import (
	"bufio"
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	gerberaImage = "gerbera/gerbera:3.3.0"
	gerberaReady = "The Web UI can be reached"

	miniDlnaContext = "../../scripts/containers/minidlna"
	miniDlnaReady   = "HTTP listening on port 8200"

	upnpClientContext   = "../../scripts/containers/upnp-client"
	upnpClientListening = "advertisement:On connect"

	startupTimeout = 60 * time.Second
	jsonLinePrefix = "{"

	UpnpClientStopTimeout = time.Second
)

func InitializeGerbera(t *testing.T) (*testcontainers.DockerContainer, netip.Addr, error) {
	return initialize(t, gerberaImage,
		testcontainers.WithWaitStrategy(
			wait.ForLog(gerberaReady).WithStartupTimeout(startupTimeout),
		),
	)
}

func InitializeMiniDlna(t *testing.T) (*testcontainers.DockerContainer, netip.Addr, error) {
	return initialize(t, "",
		testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context: miniDlnaContext,
		}),
		testcontainers.WithWaitStrategy(
			wait.ForLog(miniDlnaReady).WithStartupTimeout(startupTimeout),
		),
	)
}

func InitializeUpnpClientListener(t *testing.T) (*testcontainers.DockerContainer, error) {
	return run(t, "",
		testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context: upnpClientContext,
		}),
		testcontainers.WithCmd("--debug", "advertisements"),
		testcontainers.WithWaitStrategy(
			wait.ForLog(upnpClientListening).WithStartupTimeout(startupTimeout),
		),
	)
}

func RunUpnpClientSearch(t *testing.T, target string, timeout time.Duration) ([]map[string]string, error) {
	ctx := t.Context()

	client, err := run(t, "",
		testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context: upnpClientContext,
		}),
		testcontainers.WithCmd("--strict", "--timeout", strconv.Itoa(int(timeout/time.Second)), "search", "--search_target", target),
		testcontainers.WithWaitStrategy(
			wait.ForExit().WithExitTimeout(timeout+startupTimeout),
		),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := client.Terminate(ctx, testcontainers.StopTimeout(UpnpClientStopTimeout)); err != nil {
			t.Errorf("Failed to terminate upnp-client container: %v", err)
		}
	}()

	return UpnpClientMessages(t, client)
}

func UpnpClientMessages(t *testing.T, client *testcontainers.DockerContainer) ([]map[string]string, error) {
	logs, err := client.Logs(t.Context())
	if err != nil {
		return nil, err
	}
	defer logs.Close()

	messages := make([]map[string]string, 0)
	scanner := bufio.NewScanner(logs)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, jsonLinePrefix) {
			continue
		}

		var message map[string]string
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			return nil, err
		}

		messages = append(messages, message)
	}

	return messages, scanner.Err()
}

func DockerGatewayAddress(t *testing.T) (netip.Addr, error) {
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return netip.Addr{}, err
	}
	defer provider.Close()

	gateway, err := provider.GetGatewayIP(t.Context())
	if err != nil {
		return netip.Addr{}, err
	}

	return netip.ParseAddr(gateway)
}

func initialize(t *testing.T, image string, options ...testcontainers.ContainerCustomizer) (*testcontainers.DockerContainer, netip.Addr, error) {
	container, err := run(t, image, options...)
	if err != nil {
		return container, netip.Addr{}, err
	}

	address, err := container.ContainerIP(t.Context())
	if err != nil {
		return container, netip.Addr{}, err
	}

	parsed, err := netip.ParseAddr(address)

	return container, parsed, err
}

func run(t *testing.T, image string, options ...testcontainers.ContainerCustomizer) (*testcontainers.DockerContainer, error) {
	return testcontainers.Run(t.Context(), image, options...)
}
