package main

import (
	"context"
	"log"
	"time"

	"github.com/samthor/daikinac"
	"golang.org/x/sync/errgroup"
)

type DaikinValues struct {
	Power    *daikinac.ControlPower `json:"power,omitzero"`
	Mode     *daikinac.Mode         `json:"mode,omitzero"`
	FanRate  *daikinac.FanRate      `json:"fanRate,omitzero"`
	SetTemp  *float64               `json:"setTemp,omitzero"`
	HomeTemp *float64               `json:"homeTemp,omitzero"`
}

func runDaikin(ctx context.Context, device daikinac.Device, s *DaikinValues) (v DaikinValues, err error) {
	var si daikinac.SensorInfo
	var ci daikinac.ControlInfo

	start := time.Now()
	eg, groupCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return device.Do(groupCtx, "/aircon/get_sensor_info", nil, &si)
	})
	eg.Go(func() error {
		return device.Do(groupCtx, "/aircon/get_control_info", nil, &ci)
	})

	err = eg.Wait()
	if err != nil {
		return v, err
	}
	since := time.Since(start)
	log.Printf("fetching AC info took %v", since)

	if s != nil {
		if s.Power != nil {
			ci.Power = *s.Power
		}
		if s.Mode != nil && *s.Mode != ci.Mode {
			ci.Mode = *s.Mode

			imode := int(ci.Mode)
			if imode >= 0 && int(imode) < len(ci.PriorModes) {
				// copy values from mode we're going to
				prev := ci.PriorModes[imode]
				ci.PrimaryControl.ControlInfoMode = prev
			}
		}
		if s.Mode != nil {
			ci.Mode = *s.Mode
		}
		if s.FanRate != nil {
			ci.FanRate = *s.FanRate
		}
		if s.SetTemp != nil {
			ci.SetTemp = *s.SetTemp
		}

		err = device.Do(ctx, "/aircon/set_control_info", &ci, nil)
		if err != nil {
			return v, err
		}
	}

	return DaikinValues{
		Power:    &ci.Power,
		SetTemp:  &ci.SetTemp,
		HomeTemp: &si.HomeTemp,
		Mode:     &ci.Mode,
		FanRate:  &ci.FanRate,
	}, nil
}
