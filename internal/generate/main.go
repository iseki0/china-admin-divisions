package main

import (
	division "github.com/iseki0/china-admin-divisions"
	"os"
	"runtime"
)

func main() {
	if runtime.Version() != "go1.27.2" {
		panic("reproducible generation requires Go 1.27.2; set GOTOOLCHAIN=go1.27.2")
	}
	data, err := os.ReadFile("data.json")
	if err != nil {
		panic(err)
	}
	compressed, err := division.CompileJSON(data)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("data.bin.gz", compressed, 0644); err != nil {
		panic(err)
	}
}
