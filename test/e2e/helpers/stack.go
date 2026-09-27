package helpers

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// CurrentStack is the stack the integration run started; nil otherwise.
var CurrentStack *Stack

// Stack is the production-like environment the integration run tests
// against (INTEGRATION=1): the app image built from our Dockerfile, running
// as a web and a worker container against real PostgreSQL and NATS
// JetStream containers — the same images production runs. The test code
// starts, wires and removes all of it; there is no compose file.
type Stack struct {
	// BaseURL is the web container, as seen from the test.
	BaseURL string
	// DB is the stack's database, as seen from the test, for cases that set
	// up data directly.
	DB *sql.DB
	// FakeLLMURL is the fake LLM as the containers see it.
	FakeLLMURL string

	ctx      context.Context
	net      *testcontainers.DockerNetwork
	env      map[string]string
	fakePort int
	worker   testcontainers.Container
	all      []testcontainers.Container
}

const (
	// PostgresImage and NATSImage are the images production runs.
	PostgresImage = "postgres:18"
	NATSImage     = "nats:2.15.0"

	appImageRepo = "memolang-integration"
	appImageTag  = "latest"

	dbUser, dbPassword, dbName = "memolang", "memolang", "memolang"

	// workerAckWait is short so a killed worker's job comes back quickly in
	// the worker-death test.
	workerAckWait = "5s"
)

// StartStack builds the app image and starts the stack. fakeLLMPort is the
// port of the fake LLM running in the test process; the containers reach it
// through testcontainers' host access.
func StartStack(ctx context.Context, fakeLLMPort int) (*Stack, error) {
	s := &Stack{ctx: ctx, fakePort: fakeLLMPort}
	if err := s.start(); err != nil {
		s.Stop()
		return nil, err
	}
	return s, nil
}

func (s *Stack) start() error {
	net, err := network.New(s.ctx)
	if err != nil {
		return fmt.Errorf("network: %w", err)
	}
	s.net = net

	pg, err := s.run(testcontainers.ContainerRequest{
		Image: PostgresImage,
		Env: map[string]string{
			"POSTGRES_USER": dbUser, "POSTGRES_PASSWORD": dbPassword, "POSTGRES_DB": dbName,
		},
		ExposedPorts:   []string{"5432/tcp"},
		NetworkAliases: map[string][]string{net.Name: {"postgres"}},
		WaitingFor:     wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(2 * time.Minute),
	})
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}

	if _, err := s.run(testcontainers.ContainerRequest{
		Image:          NATSImage,
		Cmd:            []string{"-js"},
		NetworkAliases: map[string][]string{net.Name: {"nats"}},
		WaitingFor:     wait.ForLog("Server is ready").WithStartupTimeout(time.Minute),
	}); err != nil {
		return fmt.Errorf("nats: %w", err)
	}

	s.env = map[string]string{
		"DATABASE_URL":  fmt.Sprintf("postgres://%s:%s@postgres:5432/%s?sslmode=disable", dbUser, dbPassword, dbName),
		"NATS_URL":      "nats://nats:4222",
		"JOBS_ACK_WAIT": workerAckWait,
		"GIN_MODE":      "release",
	}
	s.FakeLLMURL = "http://" + testcontainers.HostInternal + ":" + strconv.Itoa(s.fakePort)

	root, err := repoRoot()
	if err != nil {
		return err
	}
	web, err := s.run(testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    root,
			Dockerfile: "Dockerfile",
			Repo:       appImageRepo,
			Tag:        appImageTag,
			KeepImage:  true,
		},
		Env:             s.env,
		ExposedPorts:    []string{"8080/tcp"},
		HostAccessPorts: []int{s.fakePort},
		WaitingFor:      wait.ForHTTP("/login").WithPort("8080/tcp").WithStartupTimeout(3 * time.Minute),
	})
	if err != nil {
		return fmt.Errorf("web: %w", err)
	}
	if s.worker, err = s.StartWorker(); err != nil {
		return err
	}

	if s.BaseURL, err = web.PortEndpoint(s.ctx, "8080/tcp", "http"); err != nil {
		return err
	}
	pgHostPort, err := pg.PortEndpoint(s.ctx, "5432/tcp", "")
	if err != nil {
		return err
	}
	s.DB, err = sql.Open("pgx", fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", dbUser, dbPassword, pgHostPort, dbName))
	return err
}

// StartWorker starts a worker container from the app image.
func (s *Stack) StartWorker() (testcontainers.Container, error) {
	w, err := s.run(testcontainers.ContainerRequest{
		Image:           appImageRepo + ":" + appImageTag,
		Cmd:             []string{"./memolang", "worker"},
		Env:             s.env,
		ExposedPorts:    []string{"8081/tcp"},
		HostAccessPorts: []int{s.fakePort},
		WaitingFor:      wait.ForHTTP("/healthz").WithPort("8081/tcp").WithStartupTimeout(time.Minute),
	})
	if err != nil {
		return nil, fmt.Errorf("worker: %w", err)
	}
	return w, nil
}

// KillWorker kills the running worker at once, like a crashed process: no
// SIGTERM grace, so its jobs are neither finished nor handed back.
func (s *Stack) KillWorker() error {
	zero := time.Duration(0)
	return s.worker.Stop(s.ctx, &zero)
}

// ReplaceWorker starts a new worker in place of the killed one.
func (s *Stack) ReplaceWorker() error {
	w, err := s.StartWorker()
	if err != nil {
		return err
	}
	s.worker = w
	return nil
}

func (s *Stack) run(req testcontainers.ContainerRequest) (testcontainers.Container, error) {
	req.Networks = []string{s.net.Name}
	c, err := testcontainers.GenericContainer(s.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if c != nil {
		s.all = append(s.all, c)
	}
	if err != nil && c != nil {
		dumpLogs(s.ctx, c)
	}
	return c, err
}

// Stop removes every container and the network.
func (s *Stack) Stop() {
	if s.DB != nil {
		s.DB.Close()
	}
	for i := len(s.all) - 1; i >= 0; i-- {
		s.all[i].Terminate(context.Background())
	}
	if s.net != nil {
		s.net.Remove(context.Background())
	}
}

// dumpLogs prints a container's output when it failed to start.
func dumpLogs(ctx context.Context, c testcontainers.Container) {
	logs, err := c.Logs(ctx)
	if err != nil {
		return
	}
	defer logs.Close()
	fmt.Fprintln(os.Stderr, "--- container logs:")
	io.Copy(os.Stderr, logs)
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("go.mod not found above %s", wd)
}
