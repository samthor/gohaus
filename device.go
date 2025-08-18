package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

type pahoWrap struct {
	c      *autopaho.ConnectionManager
	router *paho.StandardRouter
	ctx    context.Context
}

type devicePacket struct {
	payload []byte
	get     bool
}

// HandlerFunc is used to handle a z2m-like virtual node.
// It is called on a "/get" or a "/set", and the passed readSet method must be called to check if there is a Set which must be enacted.
// This is a func as the Set may arrive 'late'.
type HandlerFunc[Set, Read any] func(readSet func() (out *Set)) (Read, error)

func Register[Set, Read any](pw *pahoWrap, topic string, handler HandlerFunc[Set, Read]) {
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
		log.Fatalf("failed to subscrive to topic=%v err=%v", topicAll, err)
	}

	sender := func(readSet func() *Set) (err error) {
		out, err := handler(readSet)
		if err != nil {
			log.Printf("failed to operate on topic=%v err=%v", topic, err)
			return err // "valid" err
		}

		// failure to Marshal/Publish are fatal problems
		payload, err := json.Marshal(out)
		if err != nil {
			log.Fatalf("couldn't JSON-encode output err=%v", err)
		}
		_, err = pw.c.Publish(pw.ctx, &paho.Publish{Topic: topic, Payload: payload})
		if err != nil {
			log.Fatalf("failed to publish err=%v", err)
		}
		return nil
	}
	go runner(ch, sender)
}

func runner[Set any](packetCh <-chan devicePacket, handler func(readSet func() *Set) (err error)) {
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

		ch := neverCh
		if pending {
			ch = tokenCh
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
			err := handler(readSet)
			if err != nil {
				log.Printf("failed err=%v", err)
			}
			tokenCh <- true // return token
		}()
	}
}
