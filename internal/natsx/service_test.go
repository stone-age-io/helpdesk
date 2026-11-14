package natsx

import (
	"errors"
	"net"
	"testing"

	"github.com/nats-io/nats.go"
)

// noServer refuses every dial, so the connection never reaches a server.
type noServer struct{}

func (noServer) Dial(_, _ string) (net.Conn, error) { return nil, errors.New("no server") }

// Registration needs no reachable server and carries identity only: the name,
// a host and no endpoints. Close stops it.
func TestRegisterService(t *testing.T) {
	nc, err := nats.Connect("nats://127.0.0.1:4222", nats.SetCustomDialer(noServer{}), nats.RetryOnFailedConnect(true))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	c := &Conn{NC: nc}
	c.RegisterService()
	if c.svc == nil {
		t.Fatal("RegisterService did not register")
	}

	info := c.svc.Info()
	if info.Name != ServiceName {
		t.Errorf("name = %q, want %q", info.Name, ServiceName)
	}
	if len(info.Endpoints) != 0 {
		t.Errorf("endpoints = %v, want none", info.Endpoints)
	}
	if _, ok := info.Metadata["host"]; !ok {
		t.Errorf("metadata = %v, want a host", info.Metadata)
	}

	if err := c.svc.Stop(); err != nil {
		t.Errorf("stop: %v", err)
	}
	if !c.svc.Stopped() {
		t.Error("service still running after Stop")
	}
}
