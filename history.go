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

type HistoryReq struct {
	Paho        *pahoWrap
	Topic       string
	MinDuration time.Duration
	Ch          chan<- HistoryPacket
	GetKey      string
}

func History(req *HistoryReq) {
	var lock sync.Mutex
	var lastWrite time.Time

	packetHandler := func(p *paho.Publish) {
		now := time.Now()

		lock.Lock()
		defer lock.Unlock()

		// create packet
		out := HistoryPacket{Topic: req.Topic, When: now.Unix()}
		err := json.Unmarshal(p.Payload, &out.Packet)
		if err != nil {
			log.Fatalf("couldn't decode mqtt packet: %v", err)
		}

		// delete non-aggregatable
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

		// filter (after log)
		since := now.Sub(lastWrite)
		if since < (req.MinDuration / 2) {
			return // ignore if <50%
		}

		lastWrite = now
		req.Ch <- out
	}

	req.Paho.router.RegisterHandler(req.Topic, func(p *paho.Publish) { go packetHandler(p) })

	if req.GetKey == "-" {
		return // cannot request
	}

	ctx := context.Background()
	topicGet := fmt.Sprintf("%s/get", req.Topic) // TODO: what about + *

	sendPayload := []byte(`{}`)
	if req.GetKey != "" {
		data := map[string]string{req.GetKey: ""}
		sendPayload, _ = json.Marshal(data)
	}

	send := func() {
		_, err := req.Paho.c.Publish(ctx, &paho.Publish{
			Topic:   topicGet,
			Payload: sendPayload,
		})
		if err != nil {
			log.Fatalf("could not send get: %v", err)
		}
	}

	t := time.NewTicker(req.MinDuration)
	go func() {
		for range t.C {
			send()
		}
	}()
}
