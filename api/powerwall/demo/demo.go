package main

import (
	"context"
	"fmt"
	"flag"
	"log"
	"math"
	"time"

	"github.com/samthor/gohaus/api/powerwall"
)

var (
	flagTeslaSecret = flag.String("gw_pw", "", "Powerwall secret")
)

func main() {
	flag.Parse()

	status, err := powerwall.GetSimpleStatus(context.Background(), &powerwall.TEDApi{Secret: *flagTeslaSecret})
	if err != nil {
		log.Fatalf("could not read status: %v", err)
	}

	batteryDuration := "effectively idle"

	batteryIsCharging := status.PowerBattery < 0.0
	siteIsExporting := status.PowerSite < 0.0

	var batterySuffix string
	if batteryIsCharging {
		batterySuffix = " (charging)"
	}

	if math.Abs(status.PowerBattery) > 100.0 {
		if batteryIsCharging {
			uncharged := status.BatteryFullEnergy - status.BatteryEnergy
			duration := time.Duration(float64(time.Hour) * float64(uncharged) / -status.PowerBattery)
			batteryDuration = fmt.Sprintf("%v until full", duration.Round(time.Second))
		} else {
			duration := time.Duration(float64(time.Hour) * float64(status.BatteryEnergy) / status.PowerBattery)
			batteryDuration = fmt.Sprintf("%v until empty", duration.Round(time.Second))
		}
	}

	var siteSuffix string
	if siteIsExporting {
		siteSuffix = " (exporting)"
	}

	log.Printf("")
	log.Printf("System (Island=%v, Shutdown=%v)", status.Island, status.Shutdown)
	log.Printf("Battery: %.2f%% (%.2f / %.2f kWh), %s", float64(status.BatteryEnergy)/float64(status.BatteryFullEnergy)*100.0, float64(status.BatteryEnergy)/1000.0, float64(status.BatteryFullEnergy)/1000.0, batteryDuration)
	log.Printf("")
	log.Printf("SOLAR   %6.2f kW", status.PowerSolar/1000.0)
	log.Printf("LOAD    %6.2f kW", status.PowerLoad/1000.0)
	log.Printf("GATE    %6.2f kW%s", status.PowerSite/1000.0, siteSuffix)
	log.Printf("BATTERY %6.2f kW%s", status.PowerBattery/1000.0, batterySuffix)
	log.Printf("")
}
