// Package sysbench measures the time taken for basic system operations
// related to Go builds.
package sysbench

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"testing"
)

var (
	data1M      = make([]byte, 1<<20)
	dataFile    string
	scratchFile string
	jsonData    []byte
	jsonObject  interface{}
)

func TestMain(m *testing.M) {
	var code int
	defer os.Exit(code)

	flag.Parse()

	f, err := ioutil.TempFile("", "one-mb-of-zeroes")
	if err != nil {
		log.Fatal("could not create temp test file", err)
	}
	dataFile = f.Name()
	f.Close()
	defer os.Remove(dataFile)
	if err := ioutil.WriteFile(dataFile, data1M, 0666); err != nil {
		log.Fatal(err)
	}

	f, err = ioutil.TempFile("", "scratch")
	if err != nil {
		log.Fatal("could not create scatch file", err)
	}
	scratchFile = f.Name()
	f.Close()
	defer os.Remove(scratchFile)

	jsonData, err = exec.Command("go", "list", "-json", "fmt").Output()
	if err != nil {
		log.Fatal("failed to run go list", err)
	}
	jsonObject = make(map[string]interface{})
	if err := json.Unmarshal(jsonData, &jsonObject); err != nil {
		log.Fatal("failed to parse go list output", err)
	}

	code = m.Run()
}

func BenchmarkStat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := os.Stat(dataFile); err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkRead(b *testing.B) {
	for _, sz := range []int{1024, 50 * 1024, 1024 * 1024} {
		sz := sz
		buf := make([]byte, sz)
		name := fmt.Sprintf("%dK", sz/1024)
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				f, err := os.Open(dataFile)
				if err != nil {
					b.Fatal(err)
				}
				_, err = io.ReadFull(f, buf)
				f.Close()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkWrite(b *testing.B) {
	for _, sz := range []int{1024, 50 * 1024, 1024 * 1024} {
		sz := sz
		name := fmt.Sprintf("%dK", sz/1024)
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if err := ioutil.WriteFile(scratchFile, data1M[:sz], 0666); err != nil {
					b.Error(err)
				}
			}
		})
	}
}

func BenchmarkImport(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := build.Import("fmt", ".", 0); err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkUnmarshalJSON(b *testing.B) {
	for i := 0; i < b.N; i++ {
		out := make(map[string]interface{})
		if err := json.Unmarshal(jsonData, &out); err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkMarshalJSON(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(jsonObject); err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkSHA256(b *testing.B) {
	for _, sz := range []int{1024, 50 * 1024, 1024 * 1024} {
		sz := sz
		name := fmt.Sprintf("%dK", sz/1024)
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				sha256.Sum256(data1M[:sz])
			}
		})
	}
}
