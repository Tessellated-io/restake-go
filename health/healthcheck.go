package health

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
}

func NewHealthCheckClient(network, uuid string) *HealthCheckClient {
	return &HealthCheckClient{
		network: network,
		uuid:    uuid,
	}
}

func (hm *HealthCheckClient) Start(message string) bool {
	fmt.Println("🩺 Starting health on", hm.network)
	return hm.ping(Start, message)
}

func (hm *HealthCheckClient) Success(message string) bool {
	fmt.Println("❤️  Health success on", hm.network)
	return hm.ping(Success, message)
}

func (hm *HealthCheckClient) Failed(message string) bool {
	fmt.Printf("\u200d Health failed on %s!", hm.network)
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
		fmt.Print("Failed to ping for type", ptype, "on network", hm.network, ". Failed with", resp.StatusCode)
		return false
	}
}
