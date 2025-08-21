package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/samthor/daikinac"
	"github.com/samthor/gohaus/api/powerwall"
)

var (
	flagURL         = flag.String("url", "mqtt://mqtt.haus.samthor.au:1883", "mqtt url to connect to")
	flagTeslaSecret = flag.String("gw_pw", "", "Powerwall secret")
)

func main() {
	flag.Parse()
	var err error

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
		runner := func(readSet func() (set *DaikinValues)) (DaikinValues, error) {
			return runDaikin(context.Background(), device, readSet)
		}
		Register(pw, fmt.Sprintf("virt/daikin-ac/%s", daikinID), runner)
	}

	// -- battery

	if *flagTeslaSecret != "" {
		td := &powerwall.TEDApi{Secret: *flagTeslaSecret}

		runner := func(readSet func() (out *struct{})) (powerwall.SimpleStatus, error) {
			readSet()
			status, err := powerwall.GetSimpleStatus(context.Background(), td)
			if err != nil {
				return powerwall.SimpleStatus{}, err
			}
			return *status, nil
		}
		Register(pw, "virt/powerwall", runner)
	}

	// -- virtual day/night

	Register(pw, "virt/earth3", func(readSet func() (out *struct{})) (EarthValues, error) {
		readSet()
		return EarthValues{}, nil // TODO
	})

}

type EarthValues struct {
	HourOfDay  float64 `json:"hourOfDay"`
	SunriseAt  float64 `json:"sunriseAtHourOfDay"`
	SunsetAt   float64 `json:"sunsetAtHourOfDay"`
	WholeRatio float64 `json:"wholeRatio"` // [-1,0) sunset->sunrise, (0,+1] sunrise->sunset
}
