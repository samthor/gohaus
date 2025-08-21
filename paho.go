package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net/url"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

type pahoWrap struct {
	c      *autopaho.ConnectionManager
	router *paho.StandardRouter
	ctx    context.Context
}

func connectToPaho(ctx context.Context, rawURL string) (*pahoWrap, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	router := paho.NewStandardRouter()
	//	router.SetDebugLogger(log.Default())

	cliCfg := autopaho.ClientConfig{
		ServerUrls: []*url.URL{u},

		OnConnectError: func(err error) {
			log.Fatalf("could not connect: %v", err)
		},

		OnConnectionUp: func(cm *autopaho.ConnectionManager, c *paho.Connack) {
			log.Printf("mqtt connection up")
		},

		KeepAlive: 10,

		CleanStartOnInitialConnection: true,

		ClientConfig: paho.ClientConfig{
			ClientID: fmt.Sprintf("client-%d", rand.Int32()),
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				func(pr paho.PublishReceived) (bool, error) {
					router.Route(pr.Packet.Packet())
					return true, nil
				},
			},
			OnClientError: func(err error) {
				log.Fatalf("got client err: %v", err)
			},
			OnServerDisconnect: func(d *paho.Disconnect) {
				log.Fatalf("disconnect: str=%v code=%v", d.Properties.ReasonString, d.ReasonCode)
			},
		},
	}

	c, err := autopaho.NewConnection(ctx, cliCfg) // starts process; will reconnect until context cancelled
	if err != nil {
		return nil, err
	}

	if err = c.AwaitConnection(ctx); err != nil {
		return nil, err
	}

	return &pahoWrap{
		c:      c,
		router: router,
		ctx:    ctx,
	}, nil
}
