package health

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tessellated-io/pickaxe/log"
)

type PingType string

const (
	Start   PingType = "start"
	Fail    PingType = "fail"
	Success PingType = "success"
)

// HealthCheckClient talks to HealthChecks.io
type HealthCheckClient struct {
	network string
	uuid    string

	log *log.Logger
}

func NewHealthCheckClient(network, uuid string, log *log.Logger) *HealthCheckClient {
	return &HealthCheckClient{
		network: network,
		uuid:    uuid,

		log: log,
	}
}

func (hm *HealthCheckClient) Start() error {
	hm.log.Info().Str("network", hm.network).Msg("🏥 Starting health")

	pingMessage := fmt.Sprintf("🏥 Starting health on %s", hm.network)
	return hm.ping(Start, pingMessage)
}

func (hm *HealthCheckClient) Success(message string) error {
	hm.log.Info().Str("network", hm.network).Msg("❤️  Health success")
	return hm.ping(Success, message)
}

func (hm *HealthCheckClient) Failed(err error) error {
	hm.log.Error().Err(err).Str("network", hm.network).Msg("\u200d  Health failed")

	return hm.ping(Fail, err.Error())
}

func (hm *HealthCheckClient) ping(ptype PingType, message string) error {
	url := fmt.Sprintf("https://hc-ping.com/%s", hm.uuid)
	if ptype == Fail || ptype == Start {
		url = fmt.Sprintf("https://hc-ping.com/%s/%s", hm.uuid, ptype)
	}

	data := map[string]string{
		"msg": message,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		hm.log.Error().Err(err).Msg("failed to marshal JSON data")
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		hm.log.Error().Err(err).Msg("failed to post data")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return nil
	} else {
		err := fmt.Errorf("non-200 response code from health: %d", resp.StatusCode)
		hm.log.Error().Err(err).Str("network", hm.network).Str("ping type", string(ptype)).Int("response code", resp.StatusCode).Msg("\u200d Health failed")
		return err
	}
}
