package ftplib

import (
	"testing"
)

// go test -run TestConnect
func TestConnect(t *testing.T) {
	c, err := Connect("localhost:2121", "up", "up")
	if err != nil {
		t.Error(err)
	}
	_ = c.Quit()
}

func TestConnectAnonymous(t *testing.T) {
	c, err := ConnectAnonymous("localhost:2121")
	if err != nil {
		t.Error(err)
	}
	_ = c.Quit()
}
