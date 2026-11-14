package tests

import (
	"os"
	"testing"

	"multi-tenant-hybrid-search/tests/testenv"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testenv.Terminate()
	os.Exit(code)
}
