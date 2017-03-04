package main

// Subscribe many clients, publish into channel, wait for all messages received.
// Supposed to run for channel which only have `publish` option enabled.

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifuge-mobile"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
)

func newConnection(n int) *centrifuge.Client {
	secret := "secret"

	// Application user ID.
	user := strconv.Itoa(n)

	// Current timestamp as string.
	timestamp := centrifuge.Timestamp()

	// Empty info.
	info := ""

	// Generate client token so Centrifugo server can trust connection parameters received from client.
	token := auth.GenerateClientToken(secret, user, timestamp, info)

	creds := &centrifuge.Credentials{
		User:      user,
		Timestamp: timestamp,
		Info:      info,
		Token:     token,
	}

	wsURL := "ws://localhost:8000/connection/websocket"
	c := centrifuge.New(wsURL, creds, nil, centrifuge.DefaultConfig())

	err := c.Connect()
	if err != nil {
		log.Fatalln(err)
	}
	return c
}

type throughput struct {
	msgReceived int32
	totalMsg    int
	done        chan struct{}
}

type subEventHandler struct {
	throughput *throughput
}

func (h *subEventHandler) OnMessage(sub *centrifuge.Sub, msg *centrifuge.Message) error {
	val := atomic.AddInt32(&h.throughput.msgReceived, 1)
	if val == int32(h.throughput.totalMsg) {
		close(h.throughput.done)
	}
	return nil
}

func main() {
	var wg sync.WaitGroup
	done := make(chan struct{})
	numSubscribers := 100
	numPublish := 500

	wg.Add(numSubscribers)

	t := &throughput{
		done:     done,
		totalMsg: numPublish * numSubscribers,
	}

	for i := 0; i < numSubscribers; i++ {
		time.Sleep(time.Millisecond * 10)
		go func(n int) {
			c := newConnection(n)
			events := centrifuge.NewSubEventHandler()
			events.OnMessage(&subEventHandler{t})
			c.Subscribe("test", events)
			wg.Done()
			<-done
		}(i)
	}

	wg.Wait()

	c := newConnection(numSubscribers + 1)
	sub, err := c.Subscribe("test", nil)
	if err != nil {
		log.Fatalln(err)
	}

	data := map[string]string{"input": "1"}
	dataBytes, _ := json.Marshal(data)

	started := time.Now()
	for i := 0; i < numPublish; i++ {
		sub.Publish(dataBytes)
	}
	<-done
	elapsed := time.Since(started)
	log.Printf("Total clients %d", numSubscribers)
	log.Printf("Total messages %d", t.totalMsg)
	log.Printf("Elapsed %s", elapsed)
	log.Printf("Msg/sec %d", (1000*t.totalMsg)/int(elapsed.Nanoseconds()/1000000))
	c.Close()
}
