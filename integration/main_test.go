//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	relayImage    = "gofeatureflag/go-feature-flag:v1.58.1@sha256:07642044fc75a8ce4598e80bf626122ce25f03d3c7bf0b10a7828efeecea6dae"
	relayVersion  = "1.58.1"
	evaluationKey = "goforj-public-integration-evaluation-key"
)

// relayFixture owns one real relay container for the entire package run.
type relayFixture struct {
	container testcontainers.Container
	source    *httptest.Server
	endpoint  string
	mu        sync.Mutex
	flags     map[string]any
	err       error
}

var relayPool = struct {
	once    sync.Once
	fixture relayFixture
}{}
var nextNamespace atomic.Uint64

// TestMain always removes shared services, including failures after partial startup.
func TestMain(m *testing.M) {
	code := m.Run()
	if launchDarklyPool.fixture.stop != nil {
		close(launchDarklyPool.fixture.stop)
	}
	if launchDarklyPool.fixture.container != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if err := launchDarklyPool.fixture.container.Terminate(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "LaunchDarkly container cleanup failed")
			code = 1
		}
		cancel()
	}
	if launchDarklyPool.fixture.upstream != nil {
		launchDarklyPool.fixture.upstream.Close()
	}
	if relayPool.fixture.container != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if err := relayPool.fixture.container.Terminate(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "integration container cleanup failed")
			code = 1
		}
		cancel()
	}
	if relayPool.fixture.source != nil {
		relayPool.fixture.source.Close()
	}
	os.Exit(code)
}

// relay starts a package-scoped pinned service and fails visibly when Docker is unavailable.
func relay(t *testing.T) *relayFixture {
	t.Helper()
	relayPool.once.Do(func() {
		fixture := &relayPool.fixture
		fixture.flags = map[string]any{"bootstrap": remoteFlag(false, "")}
		fixture.source = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fixture.mu.Lock()
			data, err := json.Marshal(fixture.flags)
			fixture.mu.Unlock()
			if err != nil {
				http.Error(w, "fixture encoding failed", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
		}))
		sourcePort := fixture.source.Listener.Addr().(*net.TCPAddr).Port
		// Testcontainers establishes host forwarding after readiness, so retrieval must retry startup.
		config := "server:\n  port: 1031\nfileFormat: json\nstartWithRetrieverError: true\npollingInterval: 1000\nretrievers:\n  - kind: http\n    url: http://host.testcontainers.internal:" + fmt.Sprint(sourcePort) + "/flags.json\nauthorizedKeys:\n  evaluation:\n    - " + evaluationKey + "\n"
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{
			Image: relayImage, HostAccessPorts: []int{sourcePort}, ExposedPorts: []string{"1031/tcp"}, Cmd: []string{"/go-feature-flag", "--config=/goff/goff-proxy.yaml"},
			Files: []testcontainers.ContainerFile{
				{Reader: strings.NewReader(config), ContainerFilePath: "/goff/goff-proxy.yaml", FileMode: 0644},
			}, WaitingFor: wait.ForHTTP("/health").WithPort("1031/tcp").WithStartupTimeout(time.Minute),
		}, Started: true})
		fixture.container, fixture.err = container, err
		if err != nil {
			if container != nil {
				if logs, logErr := container.Logs(ctx); logErr == nil {
					data, _ := io.ReadAll(io.LimitReader(logs, 16<<10))
					_ = logs.Close()
					fmt.Fprintln(os.Stderr, string(data))
				}
			}
			return
		}
		host, err := container.Host(ctx)
		if err != nil {
			fixture.err = err
			return
		}
		port, err := container.MappedPort(ctx, "1031/tcp")
		if err != nil {
			fixture.err = err
			return
		}
		fixture.endpoint = "http://" + host + ":" + port.Port()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, fixture.endpoint+"/health", nil)
		if err != nil {
			fixture.err = err
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			fixture.err = err
			return
		}
		defer response.Body.Close()
		if got := response.Header.Get("x-gofeatureflag-version"); got != relayVersion {
			fixture.err = fmt.Errorf("relay running version %q does not match %s", got, relayVersion)
		}
	})
	if relayPool.fixture.err != nil {
		t.Fatalf("required relay container unavailable: %v", relayPool.fixture.err)
	}
	return &relayPool.fixture
}

// remoteFlag builds native GO Feature Flag definitions rather than GoForj documents.
func remoteFlag(value bool, query string) map[string]any {
	variation := "off"
	if value {
		variation = "on"
	}
	flag := map[string]any{"variations": map[string]any{"on": true, "off": false}, "defaultRule": map[string]any{"variation": variation}}
	if query != "" {
		flag["targeting"] = []any{map[string]any{"query": query, "variation": "on"}}
	}
	return flag
}

// update publishes a complete retrieval document under a lock with isolated test keys.
func (r *relayFixture) update(t *testing.T, flags map[string]any) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, flag := range flags {
		r.flags[key] = flag
	}
}

// namespace isolates keys across repeated package runs and test invocations.
func namespace() string { return fmt.Sprintf("test-%d-", nextNamespace.Add(1)) }

// eventually waits for real asynchronous propagation with an explicit failure deadline.
func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		if check() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("backend update did not converge")
		case <-tick.C:
		}
	}
}
