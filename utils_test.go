package ftplib

import (
	"fmt"
	"os"
	"testing"
)

// go test -run TestListDetailed
func TestListDetailed(t *testing.T) {
	d, err := os.Open(".")
	if err != nil {
		t.Error(err)
	}
	items, err := d.Readdir(-1)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(string(ListDetailed(items)))
}

// go test -run TestListShort
func TestListShort(t *testing.T) {
	d, err := os.Open(".")
	if err != nil {
		t.Error(err)
	}
	items, err := d.Readdir(-1)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(string(ListShort(items)))
}
