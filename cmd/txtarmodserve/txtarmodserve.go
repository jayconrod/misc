package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/tools/txtar"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var httpAddr, dir string
	flags := flag.NewFlagSet("txtarmodserve", flag.ContinueOnError)
	flags.StringVar(&httpAddr, "http", "localhost:6939", "HTTP address and port to listen to")
	flags.StringVar(&dir, "dir", ".", "directory to serve txtar archives from")
	if err := flags.Parse(args); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "serving on %s\n", httpAddr)
	return http.ListenAndServe(httpAddr, server{dir: dir})
}

type server struct {
	dir string
}

func (s server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		writeStatus(w, http.StatusBadRequest)
		return
	}

	modPath, version, ext, err := parsePath(req.URL.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	var contentType string
	var data []byte
	switch ext {
	case "latest":
		writeStatus(w, http.StatusNotFound)
		return
	case "list":
		contentType = "text/plain"
		data, err = s.list(modPath)
	case "info":
		contentType = "application/json"
		data, err = s.info(modPath, version)
	case "mod":
		contentType = "text/plain"
		data, err = s.mod(modPath, version)
	case "zip":
		contentType = "application/zip"
		data, err = s.zip(modPath, version)
	default:
		panic("unreachable")
	}

	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	w.Header().Add("Content-Type", contentType)
	w.Write(data)
}

func (s server) list(modPath string) ([]byte, error) {
	f, err := os.OpenFile(s.dir, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names, err := f.Readdirnames(0)
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	prefix := strings.ReplaceAll(modPath, "/", "_") + "_"
	suffix := ".txt"
	for _, name := range names {
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		v := name[len(prefix) : len(name)-len(suffix)]
		if c := semver.Canonical(v); v == "" || c != v {
			continue
		}
		buf.WriteString(v)
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

func (s server) info(modPath, version string) ([]byte, error) {
	fi, err := os.Stat(s.fileName(modPath, version))
	if err != nil {
		return nil, err
	}
	info := struct {
		Version string
		Time    string
	}{
		version,
		fi.ModTime().UTC().Format(time.RFC3339),
	}
	return json.Marshal(info)
}

func (s server) mod(modPath, version string) ([]byte, error) {
	arc, err := txtar.ParseFile(s.fileName(modPath, version))
	if err != nil {
		return nil, err
	}
	for _, f := range arc.Files {
		if f.Name == "go.mod" {
			return f.Data, nil
		}
	}
	return []byte(fmt.Sprintf("module %s", modPath)), nil
}

func (s server) zip(modPath, version string) ([]byte, error) {
	arc, err := txtar.ParseFile(s.fileName(modPath, version))
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	z := zip.NewWriter(buf)
	prefix := fmt.Sprintf("%s@%s/", modPath, version)
	for _, f := range arc.Files {
		name := prefix + f.Name
		w, err := z.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err = w.Write(f.Data); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s server) fileName(modPath, version string) string {
	name := strings.ReplaceAll(modPath, "/", "_")
	return filepath.Join(s.dir, name+"_"+version+".txt")
}

func parsePath(path string) (modPath, version, ext string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("parsing path %s: %w", path, err)
		}
	}()

	if !strings.HasPrefix(path, "/") {
		return "", "", "", errors.New("does not start with '/'")
	}
	rest := path[1:]
	if strings.HasSuffix(rest, "/@latest") {
		modPath = rest[:len(rest)-len("/@latest")]
		ext = "latest"
	} else if strings.HasSuffix(rest, "/@v/list") {
		modPath = rest[:len(rest)-len("/@v/list")]
		ext = "list"
	} else {
		at := strings.Index(rest, "/@v/")
		if at < 0 {
			return "", "", "", errors.New("does not contain '@'")
		}
		modPath = rest[:at]
		rest = rest[at+len("/@v/"):]
		dot := strings.LastIndex(rest, ".")
		if dot < 0 {
			return "", "", "", errors.New("does not have extension")
		}
		version = rest[:dot]
		ext = rest[dot+1:]
		if version, err = module.UnescapeVersion(version); err != nil {
			return "", "", "", err
		}
		if version == "" {
			return "", "", "", errors.New("version is empty")
		}
		if c := semver.Canonical(version); c != version {
			return "", "", "", fmt.Errorf("version %q is not canonical", version)
		}
	}

	if modPath, err = module.UnescapePath(modPath); err != nil {
		return "", "", "", err
	}
	switch ext {
	case "info", "latest", "list", "mod", "zip":
	default:
		return "", "", "", fmt.Errorf("invalid extension %q", ext)
	}
	return modPath, version, ext, nil
}

func writeError(w http.ResponseWriter, code int, err error) {
	w.WriteHeader(code)
	fmt.Fprintf(w, "%s: %v", http.StatusText(code), err)
}

func writeStatus(w http.ResponseWriter, code int) {
	w.WriteHeader(code)
	fmt.Fprint(w, http.StatusText(code))
}
