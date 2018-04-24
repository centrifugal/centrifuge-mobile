package main

// Subscribe many clients, publish into channel, wait for all messages received.
// Supposed to run for channel which only have `publish` option enabled.

import (
	"encoding/json"
	"flag"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifuge-mobile"
)

var protobuf = flag.Bool("protobuf", false, "Use Protobuf")

// // In production you need to receive credentials from application backend.
// func credentials(n int) *centrifuge.Credentials {
// 	// Never show secret to client of your application. Keep it on your application backend only.
// 	secret := "secret"
// 	// Application user ID.
// 	user := strconv.Itoa(n)
// 	// Current timestamp as string.
// 	timestamp := centrifuge.Timestamp()
// 	// Empty info.
// 	info := ""
// 	// Generate client token so Centrifugo server can trust connection parameters received from client.
// 	token := auth.GenerateClientToken(secret, user, timestamp, info)

// 	return &centrifuge.Credentials{
// 		User:      user,
// 		Timestamp: timestamp,
// 		Info:      info,
// 		Token:     token,
// 	}
// }

func newConnection(n int) *centrifuge.Client {
	//creds := credentials(n)
	wsURL := "ws://localhost:8000/connection/websocket"
	if *protobuf {
		wsURL += "?format=protobuf"
	}
	c := centrifuge.New(wsURL, nil, centrifuge.DefaultConfig())

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

func (h *subEventHandler) OnMessage(sub *centrifuge.Sub, pub centrifuge.Pub) {
	val := atomic.AddInt32(&h.throughput.msgReceived, 1)
	if val == int32(h.throughput.totalMsg) {
		close(h.throughput.done)
	}
}

func main() {
	flag.Parse()

	var wg sync.WaitGroup
	done := make(chan struct{})
	numSubscribers := 100
	numPublish := 1000

	wg.Add(numSubscribers)

	t := &throughput{
		done:     done,
		totalMsg: numPublish * numSubscribers,
	}

	channel := "benchmark:throughput"

	for i := 0; i < numSubscribers; i++ {
		time.Sleep(time.Millisecond)
		go func(n int) {
			c := newConnection(n)
			events := centrifuge.NewSubEventHandler()
			events.OnMessage(&subEventHandler{t})
			c.SubscribeSync(channel, events)
			wg.Done()
			<-done
		}(i)
	}

	wg.Wait()

	c := newConnection(numSubscribers + 1)
	sub, _ := c.SubscribeSync(channel, nil)

	data := map[string]interface{}{
		"_id":        "5adece493c1a23736b037c52",
		"index":      2,
		"guid":       "478a00f4-19b1-4567-8097-013b8cc846b8",
		"isActive":   false,
		"balance":    "$2,199.02",
		"picture":    "http://placehold.it/32x32",
		"age":        25,
		"eyeColor":   "blue",
		"name":       "Swanson Walker",
		"gender":     "male",
		"company":    "SHADEASE",
		"email":      "swansonwalker@shadease.com",
		"phone":      "+1 (885) 410-3991",
		"address":    "768 Paerdegat Avenue, Gouglersville, Oklahoma, 5380",
		"registered": "2016-01-24T07:40:09 -03:00",
		"latitude":   -71.336378,
		"longitude":  -28.155956,
		"tags": []string{
			"magna",
			"nostrud",
			"irure",
			"aliquip",
			"culpa",
			"sint",
		},
		"greeting":      "Hello, Swanson Walker! You have 9 unread messages.",
		"favoriteFruit": "apple",
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		panic(err.Error())
	}

	semaphore := make(chan struct{}, runtime.NumCPU())
	started := time.Now()
	for i := 0; i < numPublish; i++ {
		go func() {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			err := sub.Publish(dataBytes)
			if err != nil {
				panic(err)
			}
		}()
	}
	<-done
	elapsed := time.Since(started)
	log.Printf("Total clients %d", numSubscribers)
	log.Printf("Total messages %d", t.totalMsg)
	log.Printf("Elapsed %s", elapsed)
	log.Printf("Msg/sec %d", (1000*t.totalMsg)/int(elapsed.Nanoseconds()/1000000))
	c.Close()
}
