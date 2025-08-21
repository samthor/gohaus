package powerwall

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"google.golang.org/protobuf/proto"
)

var (
	unsafeClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, Renegotiation: tls.RenegotiateFreelyAsClient},
		},
	}
)

func ptrTo[X any](x X) *X {
	return &x
}

const (
	DefaultHost = "192.168.91.1"
)

type TEDApi struct {
	DIN    string
	Secret string
	Host   string // default "192.168.91.1"

	lock        sync.Mutex
	internalDIN string // transparently fetched if DIN not provided
}

type Query struct {
	Query     string          // string graphQL query
	Signature []byte          // tesla-private-key sig: https://github.com/jasonacox/Powerwall-Dashboard/discussions/392#discussioncomment-12023958
	Vars      json.RawMessage // interpolated into $-variables in Query (likely map[string]any)
}

// Query performs a query on the device.
func (td *TEDApi) Query(ctx context.Context, q Query) (out json.RawMessage, err error) {
	return td.QueryDevice(ctx, q, "")
}

// Query performs a query on the leader, but potentially targeted at another device (e.g., follower).
func (td *TEDApi) QueryDevice(ctx context.Context, q Query, customDin string) (out json.RawMessage, err error) {
	vars := []byte("{}")
	if q.Vars != nil {
		vars, err = json.Marshal(q.Vars)
		if err != nil {
			return nil, err
		}
	}

	recipientDin := customDin
	if recipientDin == "" {
		recipientDin, err = td.getDIN(ctx)
		if err != nil {
			return nil, err
		}
	}

	pbReq := Message{
		Message: &MessageEnvelope{
			DeliveryChannel: 1,
			Sender:          &Participant{Id: &Participant_Local{Local: 1}},
			Recipient:       &Participant{Id: &Participant_Din{Din: recipientDin}},
			Payload: &QueryType{
				Send: &PayloadQuerySend{
					Num:     ptrTo(int32(2)),
					Payload: &PayloadString{Value: 1, Text: q.Query},
					Code:    q.Signature,
					B:       &StringValue{Value: string(vars)},
				},
			},
		},
		Tail: &Tail{Value: 1},
	}

	var pbRes Message
	err = td.internalMessagePost(ctx, &pbReq, &pbRes, customDin)
	if err != nil {
		return nil, err
	}

	if pbRes.Message == nil || pbRes.Message.Payload == nil || pbRes.Message.Payload.Recv == nil {
		return nil, fmt.Errorf("missing result JSON")
	}

	out = []byte(pbRes.Message.Payload.Recv.Text)
	return out, nil
}

// Config reads a config file from the device.
// Known files include "config.json".
func (td *TEDApi) Config(ctx context.Context, file string) (out []byte, err error) {
	din, err := td.getDIN(ctx)
	if err != nil {
		return nil, err
	}

	pbReq := Message{
		Message: &MessageEnvelope{
			DeliveryChannel: 1,
			Sender:          &Participant{Id: &Participant_Local{Local: 1}},
			Recipient:       &Participant{Id: &Participant_Din{Din: din}},
			Config: &ConfigType{
				Config: &ConfigType_Send{
					Send: &PayloadConfigSend{Num: 1, File: file},
				},
			},
		},
		Tail: &Tail{Value: 1},
	}

	var pbRes Message
	err = td.internalMessagePost(ctx, &pbReq, &pbRes, "")
	if err != nil {
		return nil, err
	}

	if pbRes.Message == nil || pbRes.Message.Config == nil || pbRes.Message.Config.GetRecv() == nil || pbRes.Message.Config.GetRecv().File == nil {
		return nil, fmt.Errorf("missing File response")
	}
	return []byte(pbRes.Message.Config.GetRecv().File.Text), nil
}

func (td *TEDApi) buildUrl(pathname string) (url string) {
	// use default _or_ overwritten host
	host := DefaultHost
	if td.Host != "" {
		host = td.Host
	}
	return fmt.Sprintf("https://%s%s", host, pathname)
}

// getDIN returns the user-provided DIN, or does a cached lookup on the device's DIN by making an API request.
func (td *TEDApi) getDIN(ctx context.Context) (din string, err error) {
	if td.DIN != "" {
		return td.DIN, nil
	}

	td.lock.Lock()
	defer td.lock.Unlock()

	if td.internalDIN != "" {
		return td.internalDIN, nil
	}

	body, err := td.internalRequest(ctx, "/tedapi/din", nil)
	if err != nil {
		return "", err
	} else if len(body) < 16 {
		return "", fmt.Errorf("bad DIN from API: %v", string(body))
	}

	td.internalDIN = string(body)
	log.Printf("got DIN from leader: %v", td.internalDIN)
	return td.internalDIN, nil
}

func (td *TEDApi) internalMessagePost(ctx context.Context, in *Message, out *Message, customDin string) (err error) {
	pathname := "/tedapi/v1"
	if customDin != "" {
		// used to target specific PW devices for some queries
		pathname = fmt.Sprintf("/tedapi/device/%s/v1", customDin)
	}

	b, err := proto.Marshal(in)
	if err != nil {
		return err
	}

	body, err := td.internalRequest(ctx, pathname, bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	return proto.Unmarshal(body, out)
}

func (td *TEDApi) internalRequest(ctx context.Context, pathname string, body io.Reader) (out []byte, err error) {
	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
	}

	req, err := http.NewRequestWithContext(ctx, method, td.buildUrl(pathname), body)
	if err != nil {
		return nil, err
	}
	auth := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("Tesla_Energy_Device:%s", td.Secret)))
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", auth))

	httpResp, err := unsafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		// if we get 429 or 503, the PW could be rate-limiting us
		return nil, fmt.Errorf("non-200 status: %v", httpResp.Status)
	}
	return io.ReadAll(httpResp.Body)
}
