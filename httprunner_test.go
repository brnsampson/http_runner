package httprunner_test

import (
	"testing"
	"time"
	"log"
	"net/http"
	"crypto/tls"
	"github.com/brnsampson/httprunner"
	//"gotest.tools/v3/assert"
)

type TestLoader struct {}

func (l TestLoader) Load() (TestOptions, error) {
	return TestOptions{}, nil
}

type TestOptions struct {
	loader TestLoader
}

func (o TestOptions) GetAddr() (string, error) {
	return "127.0.0.1:1443", nil
}

func (o TestOptions) GetTlsConfig() *tls.Config {
	return &tls.Config{}
}

func (o TestOptions) GetTlsEnabled() bool {
	return false
}

func (o *TestOptions) Reload() error {
	tmp, err := o.loader.Load()
	if err != nil {
		*o = tmp
	}
	return err
}

func TestNewRunner(t *testing.T) {
	logger := log.Default()
	loader := TestLoader{}
	opts := TestOptions{loader}
	mux := http.NewServeMux()
	runner := httprunner.NewRunner()
	runner.Run(mux, &opts, logger)
	time.Sleep(100 * time.Millisecond)
	runner.Shutdown()
}
