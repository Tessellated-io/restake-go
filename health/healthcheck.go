package health

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tessellated-io/restake-go/log"
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

func (hm *HealthCheckClient) Start(message string) bool {
	hm.log.Info().Str("network", hm.network).Msg("🩺 Starting health")
	return hm.ping(Start, message)
}

func (hm *HealthCheckClient) Success(message string) bool {
	hm.log.Info().Str("network", hm.network).Msg("❤️  Health success")
	return hm.ping(Success, message)
}

func (hm *HealthCheckClient) Failed(message string) bool {
	hm.log.Info().Str("network", hm.network).Msg("❤️‍🩹  Health failed")

	return hm.ping(Fail, message)
}

func (hm *HealthCheckClient) ping(ptype PingType, message string) bool {
	url := fmt.Sprintf("https://hc-ping.com/%s", hm.uuid)
	if ptype == Fail || ptype == Start {
		url = fmt.Sprintf("https://hc-ping.com/%s/%s", hm.uuid, ptype)
	}

	data := map[string]string{
		"msg": message,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		panic(fmt.Errorf("failed to marshal JSON data: %s", err))
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		panic(fmt.Errorf("failed to post data: %s", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true
	} else {
		hm.log.Error().Str("network", hm.network).Str("ping type", string(ptype)).Int("response code", resp.StatusCode).Msg("\u200d Health failed")
		return false
	}
}
