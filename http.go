package admindivision

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// HTTPOptions controls transport. Zero options force gzip, even for clients
// explicitly refusing it. DisableForceGzip enables normal encoding negotiation.
type HTTPOptions struct{ DisableForceGzip bool }

// Handler serves one complete dataset per GET, from cached gzip bytes.
// The uncompressed representation is decoded only on negotiated identity requests.
func Handler(loader *Loader, options HTTPOptions) http.Handler {
	if loader == nil {
		loader = DefaultLoader
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(405)
			return
		}
		s := loader.Current()
		data := s.gzipData
		etag := s.etag
		compressed := true
		if options.DisableForceGzip {
			w.Header().Set("Vary", "Accept-Encoding")
			gz, id := encodingQualities(strings.Join(r.Header.Values("Accept-Encoding"), ","))
			if gz <= 0 && id <= 0 {
				w.WriteHeader(406)
				return
			}
			compressed = gz > 0 && gz >= id
		}
		if compressed {
			w.Header().Set("Content-Encoding", "gzip")
		} else {
			reader, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				panic(err)
			}
			data, err = io.ReadAll(reader)
			closeErr := reader.Close()
			if err != nil {
				panic(err)
			}
			if closeErr != nil {
				panic(closeErr)
			}
			etag = fmt.Sprintf(`"%x"`, sha256.Sum256(data))
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "public, no-cache")
		w.Header().Set("ETag", etag)
		for _, line := range r.Header.Values("If-None-Match") {
			for _, tag := range strings.Split(line, ",") {
				tag = strings.TrimSpace(tag)
				if tag == "*" || strings.TrimPrefix(tag, "W/") == etag {
					w.WriteHeader(304)
					return
				}
			}
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(200)
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	})
}
func encodingQualities(header string) (float64, float64) {
	values := map[string]float64{}
	for _, item := range strings.Split(header, ",") {
		parts := strings.Split(item, ";")
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if name == "" {
			continue
		}
		q := 1.0
		for _, param := range parts[1:] {
			k, v, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && strings.EqualFold(k, "q") {
				parsed, err := strconv.ParseFloat(v, 64)
				if err != nil || !(parsed >= 0 && parsed <= 1) {
					q = 0
				} else {
					q = parsed
				}
			}
		}
		values[name] = q
	}
	gz, ok := values["gzip"]
	if !ok {
		gz = values["*"]
	}
	id, ok := values["identity"]
	if !ok {
		id = 1
		if wildcard, exists := values["*"]; exists && wildcard == 0 {
			id = 0
		}
	}
	return gz, id
}
