// Package di provides dependency injection container for go-pcap2socks.
package di

// ConfigureServices is a placeholder for future service configuration.
// Currently not used - services are initialized directly in main.go.
func ConfigureServices(config interface{}) (*Container, error) {
	return NewContainer(), nil
}
