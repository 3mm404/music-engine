//go:build windows && amd64

package asio

import (
	"fmt"
)

func availableDriverNames() ([]string, error) {
	infos, err := ListDrivers()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(infos))
	for i := range infos {
		names[i] = infos[i].Name
	}
	return names, nil
}

func openASIODriver(name string) (device, error) {
	infos, err := ListDrivers()
	if err != nil {
		return nil, err
	}
	for _, info := range infos {
		if info.Name == name {
			driver, err := Open(info)
			if err != nil {
				return nil, err
			}
			return driver, nil
		}
	}
	return nil, fmt.Errorf("driver ASIO %q no encontrado; drivers disponibles: %v", name, driverNames(infos))
}

func driverNames(infos []DriverInfo) []string {
	names := make([]string, len(infos))
	for i, info := range infos {
		names[i] = info.Name
	}
	return names
}
