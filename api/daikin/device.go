package daikin

import (
	"context"

	"github.com/samthor/daikinac"
)

type Device struct {
	Host string
	UUID string
}

func (d *Device) Run(ctx context.Context, readSet func() (set *DaikinValues)) (v DaikinValues, err error) {
	internal := daikinac.Device{Host: d.Host, UUID: d.UUID}
	return Run(ctx, internal, readSet)
}
