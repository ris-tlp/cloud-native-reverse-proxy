package testutil

import (
	"context"
	"errors"
	"net/http"
)

type MockProxy struct {
	Healthy bool
}

func (m *MockProxy) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {}

func (m *MockProxy) Check(_ context.Context) error {
	if m.Healthy {
		return nil
	}
	return errors.New("unhealthy")
}
