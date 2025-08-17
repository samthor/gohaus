package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

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

func Register[Set, Read any](pw *pahoWrap, topic string, handler func(s *Set) (Read, error)) {
	topicAll := fmt.Sprintf("%s/#", topic)

	ch := make(chan devicePacket, 1)

	go func() {
		tokenCh := make(chan bool, 1)
		tokenCh <- true

		for {
			var set *Set
			packet := <-ch
			buffer := time.After(time.Millisecond * 350)
			var expired bool

		loop:
			for {
				// log.Printf("!got packet for topic=%v packet=%+v", topic, string(packet.payload))
				if packet.payload != nil {
					if set == nil {
						var actual Set
						set = &actual
					}
					json.Unmarshal(packet.payload, set)
				}

				if !expired {
					select {
					case packet = <-ch:
						continue
					case <-buffer:
						expired = true
					}
				}
				select {
				case packet = <-ch:
					continue
				case <-tokenCh: // take token
					break loop
				}
			}

			// actually set now: we hold token
			go func() {
				defer func() {
					tokenCh <- true
				}()

				out, err := handler(set)
				if err != nil {
					log.Printf("failed to operate on topic=%v err=%v", topic, err)
					return
				}

				payload, err := json.Marshal(out)
				if err != nil {
					log.Fatalf("couldn't JSON-encode output err=%v", err)
				}

				_, err = pw.c.Publish(pw.ctx, &paho.Publish{
					Topic:   topic,
					Payload: payload,
				})
				if err != nil {
					log.Fatalf("failed to publish err=%v", err)
				}
				log.Printf("got topic=%v payload=%s (set=%+v)", topic, string(payload), set)
			}()
		}
	}()

	pw.router.RegisterHandler(topicAll, func(p *paho.Publish) {
		if strings.HasSuffix(p.Topic, "/set") {
			// we have to pass this along
			ch <- devicePacket{payload: p.Payload}
		} else if strings.HasSuffix(p.Topic, "/get") {
			// optional
			select {
			case ch <- devicePacket{get: true}:
			default:
			}
		}
	})

	_, err := pw.c.Subscribe(context.Background(), &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{Topic: topicAll, QoS: 1},
		},
	})
	if err != nil {
		panic(err)
	}
}
