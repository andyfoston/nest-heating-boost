package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

var (
	deviceIdRegexp = regexp.MustCompile(".*/([a-zA-Z0-9-_]*)$")
	ErrRateLimit   = errors.New("too many requests")
)

type ParentRelation struct {
	Parent      string `json:"parent"`
	DisplayName string `json:"displayName"`
}

type Device struct {
	Name            string           `json:"name"`
	Type            string           `json:"type"`
	Traits          Traits           `json:"traits"`
	ParentRelations []ParentRelation `json:"parentRelations"`
}

func (d *Device) IsThermostat() bool {
	return d.Type == "sdm.devices.types.THERMOSTAT"
}

func (d *Device) DeviceID() string {
	names := deviceIdRegexp.FindStringSubmatch(d.Name)
	return names[len(names)-1]
}

func (d *Device) DisplayName() string {
	for _, parent := range d.ParentRelations {
		if parent.DisplayName != "" {
			return parent.DisplayName
		}
	}
	// Default, incase displayName cannot be found
	return d.DeviceID()
}

type Devices struct {
	Devices []Device `json:"devices"`
}

func (d *Devices) GetThermostats() []Device {
	response := make([]Device, len(d.Devices))
	for _, device := range d.Devices {
		if device.IsThermostat() {
			response = append(response, device)
		}
	}
	return response
}

type TemperatureTrait struct {
	Temperature float32 `json:"ambientTemperatureCelsius"`
}

type Traits struct {
	Temperature TemperatureTrait `json:"sdm.devices.traits.Temperature"`
}

type GetTemperatureResponse struct {
	Devices []Device `json:"devices"`
}

type ExecuteCommandRequest struct {
	Command string                 `json:"command"`
	Params  map[string]interface{} `json:"params"`
}

type TemperatureResponse struct {
	Command string                 `json:"command"`
	Traits  map[string]interface{} `json:"traits"`
}

func makeApiCall(url string, method string, accessToken string, requestData io.Reader, responseObject interface{}) error {
	req, err := http.NewRequest(method, url, requestData)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimit
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got an error response from Nest: %s", body)
	}
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return err
	}
	return nil
}

func GetDevices(accessToken string) (*Devices, error) {
	url := fmt.Sprintf("https://smartdevicemanagement.googleapis.com/v1/enterprises/%s/devices", projectID)
	response := Devices{}
	err := makeApiCall(url, "GET", accessToken, nil, &response)
	if err != nil {
		switch err {
		case ErrRateLimit:
			// `response` will be an empty Devices instance
			return &response, err
		default:
			return nil, err
		}
	}
	return &response, nil
}

func GetTemperature(accessToken string, deviceID string) (*float32, error) {
	url := fmt.Sprintf("https://smartdevicemanagement.googleapis.com/v1/enterprises/%s/devices/%s", projectID, deviceID)
	// TODO rename
	response := TemperatureResponse{}
	err := makeApiCall(url, "GET", accessToken, nil, &response)
	if err != nil {
		return nil, err
	}
	setpoint := response.Traits["sdm.devices.traits.ThermostatTemperatureSetpoint"]
	if temperature, ok := setpoint.(map[string]interface{}); ok {
		temp := float32(temperature["heatCelsius"].(float64))
		return &temp, nil
	}
	//if output, ok := temp.(float32); ok {
	//	return &output, nil
	//}
	return nil, fmt.Errorf("failed to parse result from Nest: %s - %s", setpoint, response.Traits)
}

func SetTemperature(accessToken, deviceID string, temperature float32) error {
	url := fmt.Sprintf("https://smartdevicemanagement.googleapis.com/v1/enterprises/%s/devices/%s:executeCommand", projectID, deviceID)
	executeCommandRequest := GetSetHeatCommandRequest(temperature)
	request, err := json.Marshal(executeCommandRequest)
	if err != nil {
		return err
	}
	err = makeApiCall(url, "POST", accessToken, bytes.NewReader(request), "")
	return err
}

func GetSetHeatCommandRequest(temperature float32) ExecuteCommandRequest {
	return ExecuteCommandRequest{
		Command: "sdm.devices.commands.ThermostatTemperatureSetpoint.SetHeat",
		Params: map[string]interface{}{
			"heatCelsius": temperature,
		},
	}
}
