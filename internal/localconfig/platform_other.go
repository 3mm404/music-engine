//go:build !windows

package localconfig

import (
	"errors"
	"io"
)

var errPlatform = errors.New("la configuracion persistente requiere Windows x64")

func Prepare(string) error             { return errPlatform }
func Lock(string) (io.Closer, error)   { return nil, errPlatform }
func protect([]byte) ([]byte, error)   { return nil, errPlatform }
func unprotect([]byte) ([]byte, error) { return nil, errPlatform }
func replace(string, string) error     { return errPlatform }
func ReadSecret() (string, error)      { return "", errPlatform }
