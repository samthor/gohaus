package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/paho"
)

type HistoryPacket struct {
	Topic  string         `json:"-"`
	When   int64          `json:"n"` // seconds
	Packet map[string]any `json:"p"`
}

func History(pw *pahoWrap, topic string, minDuration time.Duration, ch chan<- HistoryPacket) {
	var lock sync.Mutex
	var lastWrite time.Time

	packetHandler := func(p *paho.Publish) {
		lock.Lock()
		defer lock.Unlock()

		now := time.Now()
		since := now.Sub(lastWrite)
		if since < (minDuration / 2) {
			return // ignore if <50%
		}

		out := HistoryPacket{Topic: topic}
		err := json.Unmarshal(p.Payload, &out.Packet)
		if err != nil {
			log.Fatalf("couldn't decode mqtt packet: %v", err)
		}

		for key, value := range out.Packet {
			_, ok := value.(float64)
			if ok {
				continue
			}
			_, ok = value.(bool)
			if ok {
				continue
			}
			delete(out.Packet, key)
		}

		out.When = now.Unix()
		lastWrite = now
		ch <- out
	}

	pw.router.RegisterHandler(topic, func(p *paho.Publish) { go packetHandler(p) })

	ctx := context.Background()
	topicGet := fmt.Sprintf("%s/get", topic) // TODO: what about + *

	send := func() {
		_, err := pw.c.Publish(ctx, &paho.Publish{
			Topic:   topicGet,
			Payload: []byte("{}"), // z2m needs "request"
		})
		if err != nil {
			log.Fatalf("could not send get: %v", err)
		}
	}

	t := time.NewTicker(minDuration)
	go func() {
		for range t.C {
			send()
		}
	}()
}
