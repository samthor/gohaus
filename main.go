package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net/url"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/samthor/daikinac"
)

func main() {
	devices := map[string]daikinac.Device{
		"den":         {Host: "192.168.3.146"},
		"living-room": {Host: "192.168.3.152"},
		"bedroom":     {Host: "192.168.3.204"},
		"loft":        {Host: "192.168.3.225"},
		"office":      {Host: "192.168.3.245", UUID: "f45aab28604811eca7c4737954d1686f"},
	}

	u, err := url.Parse("mqtt://mqtt.haus.samthor.au:1883")
	if err != nil {
		panic(err)
	}

	router := paho.NewStandardRouter()
	//	router.SetDebugLogger(log.Default())

	cliCfg := autopaho.ClientConfig{
		ServerUrls: []*url.URL{u},

		OnConnectError: func(err error) {
			log.Fatalf("could not connect: %v", err)
		},

		OnConnectionUp: func(cm *autopaho.ConnectionManager, c *paho.Connack) {
			log.Printf("conn up")
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

	c, err := autopaho.NewConnection(context.Background(), cliCfg) // starts process; will reconnect until context cancelled
	if err != nil {
		panic(err)
	}

	if err = c.AwaitConnection(context.Background()); err != nil {
		panic(err)
	}

	pw := &pahoWrap{
		c:      c,
		router: router,
		ctx:    context.Background(),
	}

	// _, err = pw.c.Subscribe(context.Background(), &paho.Subscribe{
	// 	Subscriptions: []paho.SubscribeOptions{
	// 		{Topic: "#", QoS: 0},
	// 	},
	// })
	// if err != nil {
	// 	panic(err)
	// }
	for daikinID, device := range devices {
		Register(pw, fmt.Sprintf("virt/daikin-ac/%s", daikinID), func(s *DaikinValues) (DaikinValues, error) {
			return runDaikin(pw.ctx, device, s)
		})
	}

	log.Printf("connected?")
	time.Sleep(time.Second * 200)
}
