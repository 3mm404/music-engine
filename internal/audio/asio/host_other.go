//go:build !windows || !amd64

package asio

import "errors"

type Driver struct{}

func availableDriverNames() ([]string, error) {
	return nil, errors.New("ASIO solo está disponible en Windows amd64")
}

func openASIODriver(string) (device, error) {
	return nil, errors.New("ASIO solo está disponible en Windows amd64")
}

func (*Driver) Stop() error  { return nil }
func (*Driver) Close() error { return nil }
func (*Driver) CanSampleRate(float64) error {
	return errors.New("ASIO solo está disponible en Windows amd64")
}
