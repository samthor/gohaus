package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"time"

	"github.com/samthor/daikinac"
	"github.com/samthor/gohaus/api/powerwall"
)

var (
	flagURL = flag.String("url", "mqtt://mqtt.haus.samthor.au:1883", "mqtt url to connect to")
)

func main() {
	flag.Parse()
	var err error

	td := &powerwall.TEDApi{
		Secret: "TWDTPKHSGR",
	}
	err = powerwall.GetConfig(context.Background(), td)
	if err != nil {
		log.Fatalf("couldn't fetch Tesla config: %v", err)
	}

	err = powerwall.GetStatus(context.Background(), td)
	if err != nil {
		log.Fatalf("couldn't fetch Tesla status: %v", err)
	}

	paho, err := connectToPaho(context.Background(), *flagURL)
	if err != nil {
		log.Fatalf("could not connectToPaho url=%v err=%v", *flagURL, err)
	}

	config(context.Background(), paho)
	<-make(chan bool) // sleep forever
}

func config(ctx context.Context, pw *pahoWrap) {
	_ = ctx

	// -- daikin ACs

	devices := map[string]daikinac.Device{
		"den":         {Host: "192.168.3.146"},
		"living-room": {Host: "192.168.3.152"},
		"bedroom":     {Host: "192.168.3.204"},
		"loft":        {Host: "192.168.3.225"},
		"office":      {Host: "192.168.3.245", UUID: "f45aab28604811eca7c4737954d1686f"},
	}

	for daikinID, device := range devices {
		build := func(announce func(DaikinValues)) (HandlerFunc[DaikinValues, DaikinValues], error) {
			// cannot announce, just returns runner
			return func(readSet func() (set *DaikinValues)) (DaikinValues, error) {
				return runDaikin(context.Background(), device, readSet)
			}, nil
		}
		Register(pw, fmt.Sprintf("virt/daikin-ac/%s", daikinID), build)
	}

	// -- virtual day/night

	Register(pw, "virt/earth3", func(announce func(EarthValues)) (HandlerFunc[struct{}, EarthValues], error) {
		go func() {
			time.Sleep(time.Millisecond * time.Duration(4000+rand.Int64N(8000)))
		}()

		return func(readSet func() (out *struct{})) (EarthValues, error) {
			return EarthValues{}, nil
		}, nil
	})

}

type EarthValues struct {
	HourOfDay  float64 `json:"hourOfDay"`
	SunriseAt  float64 `json:"sunriseAtHourOfDay"`
	SunsetAt   float64 `json:"sunsetAtHourOfDay"`
	WholeRatio float64 `json:"wholeRatio"` // [-1,0) sunset->sunrise, (0,+1] sunrise->sunset
}
