package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/paho"
)

const (
	defaultTimeout = time.Second * 20
)

type devicePacket struct {
	payload []byte
	get     bool
}

// HandlerFunc is used to handle a z2m-like virtual node.
// It is called on a "/get" or a "/set", and the passed readSet method must be called to check if there is a Set which must be enacted.
// This is a func as the Set may arrive 'late'.
type HandlerFunc[Set, Read any] func(ctx context.Context, readSet func() (out *Set)) (Read, error)

// Register creates a virtual z2m-like virtual device rooted at the given topic.
// The handler must use the `readSet` function to check if there's data to send, otherwise it will be called forever.
func Register[Set, Read any](pw *pahoWrap, topic string, handler HandlerFunc[Set, Read]) {

	announce := func(out Read) {
		// failure to Marshal/Publish are fatal problems
		payload, err := json.Marshal(out)
		if err != nil {
			log.Fatalf("couldn't JSON-encode output for topic=%v out=%+v err=%v", topic, out, err)
		}
		_, err = pw.c.Publish(pw.ctx, &paho.Publish{Topic: topic, Payload: payload})
		if err != nil {
			log.Fatalf("failed to publish for topic=%v err=%v", topic, err)
		}
	}

	topicAll := fmt.Sprintf("%s/#", topic)
	ch := make(chan devicePacket)

	pw.router.RegisterHandler(topicAll, func(p *paho.Publish) {
		if strings.HasSuffix(p.Topic, "/set") {
			ch <- devicePacket{payload: p.Payload}
		} else if strings.HasSuffix(p.Topic, "/get") {
			ch <- devicePacket{get: true}
		}
	})

	_, err := pw.c.Subscribe(context.Background(), &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{Topic: topicAll, QoS: 1},
		},
	})
	if err != nil {
		log.Fatalf("failed to subscrive to topicAll=%v err=%v", topicAll, err)
	}

	sender := func(readSet func() *Set) {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		out, err := handler(timeoutCtx, readSet)
		if err != nil {
			log.Printf("failed to operate on topic=%v err=%v", topic, err)
			return // "valid" err, just failed to do thing
		}
		announce(out)
	}
	go runner(ch, sender)
}

func runner[Set any](packetCh <-chan devicePacket, handler func(readSet func() *Set)) {
	neverCh := make(chan bool)
	tokenCh := make(chan bool, 1)
	tokenCh <- true

	var lock sync.Mutex
	var pending bool
	var set *Set

	readSet := func() (out *Set) {
		lock.Lock()
		defer lock.Unlock()

		pending = false // users must invoke readSet to clear this bit
		out = set
		set = nil
		return out
	}

	var packet devicePacket

	for {
		lock.Lock()

		if packet.get || packet.payload != nil {
			pending = true
		}
		if packet.payload != nil {
			if set == nil {
				var actual Set
				set = &actual
			}
			json.Unmarshal(packet.payload, set)
		}

		ch := neverCh // we don't want to run the handler yet (will never trigger)
		if pending {
			ch = tokenCh // we do want to run the handler, wait for token
		}

		lock.Unlock()

		select {
		case packet = <-packetCh:
			continue
		case <-ch:
			packet = devicePacket{}
		}

		// token available and we're pending: kickoff task
		go func() {
			handler(readSet)
			tokenCh <- true // return token
		}()
	}
}
