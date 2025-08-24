package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/samthor/gohaus/api/daikin"
	"github.com/samthor/gohaus/api/powerwall"
)

var (
	flagHistoryPath = flag.String("history", "", "if specified, path to log history")
	flagURL         = flag.String("url", "mqtt://mqtt.haus.samthor.au:1883", "mqtt url to connect to")
	flagTeslaSecret = flag.String("gw_pw", "", "Powerwall secret")
)

var (
	daikinDevices = map[string]daikin.Device{
		"den":         {Host: "192.168.3.146"},
		"living-room": {Host: "192.168.3.152"},
		"bedroom":     {Host: "192.168.3.204"},
		"loft":        {Host: "192.168.3.225"},
		"office":      {Host: "192.168.3.245", UUID: "f45aab28604811eca7c4737954d1686f"},
	}
)

func main() {
	flag.Parse()
	var err error

	paho, err := connectToPaho(context.Background(), *flagURL)
	if err != nil {
		log.Fatalf("could not connectToPaho url=%v err=%v", *flagURL, err)
	}

	configHistory(paho)
	configDevices(paho)
	<-make(chan bool) // sleep forever
}

func writePacket(packet HistoryPacket) (err error) {
	enc := encodeTopic(packet.Topic)

	p := path.Join(*flagHistoryPath, enc)

	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0660)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(packet)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	_, err = f.Write(b)
	return err
}

func configHistory(pw *pahoWrap) {

	if *flagHistoryPath == "" {
		log.Printf("not running history")
		return
	}
	log.Printf("writing history to: %v", *flagHistoryPath)

	ch := make(chan HistoryPacket)
	go func() {
		os.MkdirAll(*flagHistoryPath, 0775)

		for packet := range ch {
			err := writePacket(packet)
			if err != nil {
				log.Fatalf("could not write packet=%+v: %v", packet, err)
			}
		}
	}()

	for daikinID := range daikinDevices {
		History(pw, fmt.Sprintf("virt/daikin-ac/%s", daikinID), time.Minute, ch)
	}
	History(pw, "virt/powerwall", time.Second*30, ch)
	History(pw, "zigbee2mqtt/device/power/rack", time.Second*30, ch)
	History(pw, "zigbee2mqtt/device/sensor/noc-etc", time.Second*30, ch)
	History(pw, "zigbee2mqtt/device/sensor/whatever", time.Second*30, ch)

}

func configDevices(pw *pahoWrap) {

	// -- daikin ACs

	for daikinID, device := range daikinDevices {
		Register(pw, fmt.Sprintf("virt/daikin-ac/%s", daikinID), device.Run)
	}

	// -- battery

	if *flagTeslaSecret != "" {
		td := &powerwall.TEDApi{Secret: *flagTeslaSecret}

		runner := func(ctx context.Context, readSet func() (out *struct{})) (powerwall.SimpleStatus, error) {
			readSet()
			status, err := powerwall.GetSimpleStatus(ctx, td)
			if err != nil {
				return powerwall.SimpleStatus{}, err
			}
			return *status, nil
		}
		Register(pw, "virt/powerwall", runner)
	}

	// -- virtual day/night

	Register(pw, "virt/earth3", func(ctx context.Context, readSet func() (out *struct{})) (EarthValues, error) {
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
