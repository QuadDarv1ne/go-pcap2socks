package upnp

import (
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type UPnP struct {
	enabled     bool
	externalIP  string
	controlURL  string
	serviceType string
	discovered  []Device
}

type Device struct {
	UDN          string
	FriendlyName string
	Manufacturer string
	ModelName    string
	ServiceType  string
	ControlURL   string
}

type root struct {
	Device device `xml:"device"`
}

type device struct {
	DeviceType   string    `xml:"deviceType"`
	FriendlyName string    `xml:"friendlyName"`
	Manufacturer string    `xml:"manufacturer"`
	ModelName    string    `xml:"modelName"`
	UDN          string    `xml:"UDN"`
	Services     []service `xml:"serviceList>service"`
	Devices      []device  `xml:"deviceList>device"`
}

type service struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

func New() *UPnP {
	return &UPnP{
		enabled: false,
	}
}

func (u *UPnP) Discover() ([]Device, error) {
	// Send M-SEARCH request
	conn, err := net.Dial("udp4", "239.255.255.250:1900")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Set read timeout
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	msg := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST:239.255.255.250:1900\r\n" +
		"ST:ssdp:all\r\n" +
		"MAN:\"ssdp:discover\"\r\n" +
		"MX:3\r\n" +
		"\r\n"

	_, err = conn.Write([]byte(msg))
	if err != nil {
		return nil, err
	}

	// Read responses
	u.discovered = []Device{}
	buf := make([]byte, 4096)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			continue
		}

		response := string(buf[:n])
		device, err := u.parseSSDPResponse(response)
		if err == nil && device != nil {
			u.discovered = append(u.discovered, *device)
		}
	}

	return u.discovered, nil
}

func (u *UPnP) parseSSDPResponse(response string) (*Device, error) {
	lines := strings.Split(response, "\r\n")
	var location string

	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), "location:") {
			location = strings.TrimSpace(line[9:])
			break
		}
	}

	if location == "" {
		return nil, fmt.Errorf("no location in SSDP response")
	}

	// Fetch device description
	return u.fetchDeviceDescription(location)
}

func (u *UPnP) fetchDeviceDescription(location string) (*Device, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(location)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return u.parseDeviceDescription(body)
}

func (u *UPnP) parseDeviceDescription(data []byte) (*Device, error) {
	var root root
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	device := &Device{
		UDN:          root.Device.UDN,
		FriendlyName: root.Device.FriendlyName,
		Manufacturer: root.Device.Manufacturer,
		ModelName:    root.Device.ModelName,
	}

	// Find WANIPConnection or WANPPPConnection service
	for _, service := range root.Device.Services {
		if strings.Contains(service.ServiceType, "WANIPConnection") ||
			strings.Contains(service.ServiceType, "WANPPPConnection") {
			device.ServiceType = service.ServiceType
			device.ControlURL = service.ControlURL
			u.controlURL = service.ControlURL
			u.serviceType = service.ServiceType
			break
		}
	}

	// Check sub-devices
	for _, subDevice := range root.Device.Devices {
		for _, service := range subDevice.Services {
			if strings.Contains(service.ServiceType, "WANIPConnection") ||
				strings.Contains(service.ServiceType, "WANPPPConnection") {
				device.ServiceType = service.ServiceType
				device.ControlURL = service.ControlURL
				u.controlURL = service.ControlURL
				u.serviceType = service.ServiceType
				break
			}
		}
	}

	return device, nil
}

func (u *UPnP) GetExternalIP() (string, error) {
	if u.controlURL == "" {
		return "", fmt.Errorf("no UPnP control URL found")
	}

	soapBody := `<?xml version="1.0"?>
	<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
		<s:Body>
			<u:GetExternalIPAddress xmlns:u="%s">
			</u:GetExternalIPAddress>
		</s:Body>
	</s:Envelope>`

	resp, err := soapRequest(u.controlURL, u.serviceType, fmt.Sprintf(soapBody, u.serviceType))
	if err != nil {
		return "", err
	}

	// Parse response to extract IP
	u.externalIP = extractIPFromResponse(resp)
	return u.externalIP, nil
}

func (u *UPnP) AddPortMapping(protocol string, externalPort, internalPort int, internalClient string, description string, leaseDuration int) error {
	if u.controlURL == "" {
		return fmt.Errorf("no UPnP control URL found")
	}

	soapBody := `<?xml version="1.0"?>
	<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
		<s:Body>
			<u:AddPortMapping xmlns:u="%s">
				<NewRemoteHost></NewRemoteHost>
				<NewExternalPort>%d</NewExternalPort>
				<NewProtocol>%s</NewProtocol>
				<NewInternalPort>%d</NewInternalPort>
				<NewInternalClient>%s</NewInternalClient>
				<NewEnabled>1</NewEnabled>
				<NewPortMappingDescription>%s</NewPortMappingDescription>
				<NewLeaseDuration>%d</NewLeaseDuration>
			</u:AddPortMapping>
		</s:Body>
	</s:Envelope>`

	body := fmt.Sprintf(soapBody, u.serviceType, externalPort, protocol, internalPort, internalClient, description, leaseDuration)
	_, err := soapRequest(u.controlURL, u.serviceType, body)
	return err
}

func (u *UPnP) DeletePortMapping(protocol string, externalPort int) error {
	if u.controlURL == "" {
		return fmt.Errorf("no UPnP control URL found")
	}

	soapBody := `<?xml version="1.0"?>
	<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
		<s:Body>
			<u:DeletePortMapping xmlns:u="%s">
				<NewRemoteHost></NewRemoteHost>
				<NewExternalPort>%d</NewExternalPort>
				<NewProtocol>%s</NewProtocol>
			</u:DeletePortMapping>
		</s:Body>
	</s:Envelope>`

	body := fmt.Sprintf(soapBody, u.serviceType, externalPort, protocol)
	_, err := soapRequest(u.controlURL, u.serviceType, body)
	return err
}

func (u *UPnP) IsEnabled() bool {
	return u.enabled
}

func (u *UPnP) SetEnabled(enabled bool) {
	u.enabled = enabled
}

func (u *UPnP) GetDiscoveredDevices() []Device {
	return u.discovered
}

func soapRequest(url, serviceType, body string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", fmt.Sprintf(`"%s#%s"`, serviceType, getActionFromType(serviceType)))

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(respBody), nil
}

func getActionFromType(serviceType string) string {
	if strings.Contains(serviceType, "WANIPConnection") {
		return "GetExternalIPAddress"
	}
	return "GetExternalIPAddress"
}

func extractIPFromResponse(resp string) string {
	// Simple extraction - in production, use proper XML parsing
	start := strings.Index(resp, "<NewExternalIPAddress>")
	if start == -1 {
		return ""
	}
	start += len("<NewExternalIPAddress>")
	end := strings.Index(resp[start:], "</NewExternalIPAddress>")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(resp[start : start+end])
}
