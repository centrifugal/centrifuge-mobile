package centrifuge

import "testing"

func TestEventHandler(t *testing.T) {
	eh := NewEventHandler()
	if eh == nil {
		t.Fatalf("nil EventHandler")
	}
}

type testConnectHandler struct{}

func (h *testConnectHandler) OnConnect(c *Client, e *ConnectEvent) {
	return
}

func TestEventProxy(t *testing.T) {
	eh := NewEventHandler()
	if eh == nil {
		t.Fatalf("nil EventHandler")
	}
	eh.OnConnect(&testConnectHandler{})
}
