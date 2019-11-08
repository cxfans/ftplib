package ftplib

import (
	"bytes"
	"fmt"
	"os"
)

var null = []byte("drwxrwxrwx 1 user group 0 Apr  1 00:00 .\r\n" +
	"drwxrwxrwx 1 user group 0 Apr  1 00:00 ..\r\n")

func ListDetailed(items []os.FileInfo) []byte {
	if len(items) == 0 {
		return null
	}
	var buf bytes.Buffer
	for _, item := range items {
		_, _ = fmt.Fprintf(&buf, "%s\t1 user\tgroup\t%8d %s %s\r\n", item.Mode(),
			item.Size(), item.ModTime().Format("Jan _2 15:04"), item.Name())
	}
	return buf.Bytes()
}

func ListShort(items []os.FileInfo) []byte {
	if len(items) == 0 {
		return null
	}
	var buf bytes.Buffer
	for _, item := range items {
		_, _ = fmt.Fprintf(&buf, "%s\r\n", item.Name())
	}
	return buf.Bytes()
}
