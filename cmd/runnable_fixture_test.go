package cmd

import (
	"os"
	"runtime"
	"testing"
)

var sharedRunnableTestPayload = loadRunnableTestPayload()

func loadRunnableTestPayload() []byte {
	if runtime.GOOS == "windows" {
		path, err := os.Executable()
		if err != nil {
			panic(err)
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		return payload
	}
	return []byte("#!/bin/sh\nexit 0\n")
}

func testRunnablePayload(t testing.TB) []byte {
	t.Helper()
	return append([]byte(nil), sharedRunnableTestPayload...)
}

func testRunnablePayloadString(t testing.TB) string {
	t.Helper()
	return string(testRunnablePayload(t))
}
