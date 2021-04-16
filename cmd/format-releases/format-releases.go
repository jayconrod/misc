package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

func main() {
	log.SetFlags(0)
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

type Version struct {
	Version string
	Stable  bool
	Files   []Download
}

type Download struct {
	Filename, OS, Arch, SHA256, Kind string
}

func run(args []string) error {
	flags := flag.NewFlagSet("format-releases", flag.ExitOnError)
	in := flags.String("in", "", "input json file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	tags := flags.Args()
	if len(tags) == 0 {
		return fmt.Errorf("no versions specified")
	}

	var data []byte
	var err error
	if *in == "" {
		r, err := http.Get("https://golang.org/dl/?mode=json&include=all")
		if err != nil {
			return err
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			return fmt.Errorf("bad status: %d %s", r.StatusCode, r.Status)
		}
		data, err = ioutil.ReadAll(r.Body)
	} else {
		data, err = ioutil.ReadFile(*in)
	}
	if err != nil {
		return err
	}

	var versions []Version
	if err := json.Unmarshal(data, &versions); err != nil {
		return err
	}

	for _, v := range versions {
		var versionFound bool
		for _, tag := range tags {
			if v.Version == tag {
				versionFound = true
				break
			}
		}
		if !versionFound {
			continue
		}

		for _, d := range v.Files {
			if d.Kind != "archive" {
				continue
			}
			if d.Arch == "armv6l" {
				d.Arch = "arm"
			}
			n, _ := fmt.Printf("        \"%s_%s\":", d.OS, d.Arch)
			fmt.Printf("%*s(%q, %q),\n", 29-n, "", d.Filename, d.SHA256)
		}
	}
	return nil
}
