package daikin

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/samthor/daikinac"
	"golang.org/x/sync/errgroup"
)

type DaikinValues struct {
	Power   *daikinac.ControlPower `json:"power,omitzero"`
	Mode    *daikinac.Mode         `json:"mode,omitzero"`
	FanRate *daikinac.FanRate      `json:"fanRate,omitzero"`
	SetTemp *float64               `json:"setTemp,omitzero"`

	// not settable

	HomeTemp    *float64 `json:"homeTemp,omitzero"`
	OutsideTemp *float64 `json:"outsideTemp,omitzero"`
}

func (dv *DaikinValues) String() string {
	var parts []string

	if dv.Power != nil {
		parts = append(parts, fmt.Sprintf("Power:%v", *dv.Power))
	}
	if dv.Mode != nil {
		parts = append(parts, fmt.Sprintf("Mode:%v", *dv.Mode))
	}
	if dv.FanRate != nil {
		parts = append(parts, fmt.Sprintf("FanRate:%v", *dv.FanRate))
	}
	if dv.SetTemp != nil {
		parts = append(parts, fmt.Sprintf("SetTemp:%v", *dv.SetTemp))
	}
	if dv.HomeTemp != nil {
		parts = append(parts, fmt.Sprintf("HomeTemp:%v", *dv.HomeTemp))
	}
	if dv.OutsideTemp != nil {
		parts = append(parts, fmt.Sprintf("OutsideTemp:%v", *dv.OutsideTemp))
	}

	return fmt.Sprintf("{%s}", strings.Join(parts, " "))
}

func Run(ctx context.Context, device daikinac.Device, readSet func() (set *DaikinValues)) (v DaikinValues, err error) {
	start := time.Now()

	var si daikinac.SensorInfo
	eg, groupCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		// TODO: put this into a cache for ~mins (temps don't change that fast)
		return device.Do(groupCtx, "/aircon/get_sensor_info", nil, &si)
	})

	var ci daikinac.ControlInfo
	err = device.Do(groupCtx, "/aircon/get_control_info", nil, &ci)
	if err != nil {
		return v, err
	}

	s := readSet()
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

	// wait for sensor info; we don't need it until end
	err = eg.Wait()
	if err != nil {
		return v, err
	}

	v = DaikinValues{
		Power:   &ci.Power,
		SetTemp: &ci.SetTemp,
		Mode:    &ci.Mode,
		FanRate: &ci.FanRate,
	}

	// TODO: some places might get to 0°C, not here
	if si.HomeTemp != 0.0 {
		v.HomeTemp = &si.HomeTemp
	}
	if si.OutsideTemp != 0.0 {
		v.OutsideTemp = &si.OutsideTemp
	}

	hasUUID := device.UUID != ""
	log.Printf("run AC uuid=%v (set=%s, read=%s) info took %v", hasUUID, s, &v, time.Since(start))

	return v, nil
}
