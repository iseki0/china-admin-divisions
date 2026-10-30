package admindivision

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"io"
	"math/rand"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"testing"
)

func uncompressed(t *testing.T, b []byte) []byte {
	t.Helper()
	r, e := gzip.NewReader(bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	v, e := io.ReadAll(r)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	return v
}
func TestDeterministicRoundtrip(t *testing.T) {
	source, e := os.ReadFile("data.json")
	if e != nil {
		t.Fatal(e)
	}
	var original []Node
	if e = json.Unmarshal(source, &original); e != nil {
		t.Fatal(e)
	}
	shuffled := append([]Node(nil), original...)
	rand.New(rand.NewSource(42)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	input, _ := json.Marshal(shuffled)
	a, e := CompileJSON(input)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(a, embedded) {
		t.Fatal("input order changed gzip bytes")
	}
	decoded, e := DecodeBinary(uncompressed(t, a))
	if e != nil {
		t.Fatal(e)
	}
	byCode := map[string]Node{}
	for _, n := range original {
		byCode[n.Code] = n
	}
	for _, n := range decoded {
		if !reflect.DeepEqual(n, byCode[n.Code]) {
			t.Fatalf("lost fields: %s", n.Code)
		}
	}
	reencoded, _ := json.Marshal(decoded)
	b, e := CompileJSON(reencoded)
	if e != nil || !bytes.Equal(a, b) {
		t.Fatal("unstable roundtrip", e)
	}
	t.Logf("records=%d json=%d binary=%d gzip=%d", len(decoded), len(source), len(uncompressed(t, a)), len(a))
}
func TestLeadingZeroCode(t *testing.T) {
	b, e := CompileJSON([]byte(`[{"code":"000001","name":"甲","pinyin":"jia","deprecated":false,"level":1,"parentCode":null}]`))
	if e != nil {
		t.Fatal(e)
	}
	nodes, e := decodeGzip(b)
	if e != nil || nodes[0].Code != "000001" {
		t.Fatal(nodes, e)
	}
}
func TestRejectInvalidTrees(t *testing.T) {
	for _, input := range []string{
		`[]`,
		`[{"code":"1","name":"甲","pinyin":"jia","deprecated":false,"level":2,"parentCode":null}]`,
		`[{"code":"1","name":"甲","pinyin":"jia","deprecated":false,"level":1,"parentCode":"2"}]`,
		`[{"code":"1","name":"甲","pinyin":"jia","level":1,"parentCode":null}]`,
		`[{"code":"1000000000000","name":"甲","pinyin":"jia","deprecated":false,"level":1,"parentCode":null}]`,
	} {
		if _, e := CompileJSON([]byte(input)); e == nil {
			t.Fatal("accepted", input)
		}
	}
}
func TestRejectCorruptBinary(t *testing.T) {
	raw := uncompressed(t, embedded)
	for _, end := range []int{0, 9, 10, len(raw) - 1} {
		if _, e := DecodeBinary(raw[:end]); e == nil {
			t.Fatal("accepted truncation", end)
		}
	}
	for _, offset := range []int{0, 4, 10 + 15} {
		b := bytes.Clone(raw)
		b[offset] = 255
		if _, e := DecodeBinary(b); e == nil {
			t.Fatal("accepted corrupt byte", offset)
		}
	}
	b := bytes.Clone(raw)
	binary.LittleEndian.PutUint32(b[18:], 0)
	if _, e := DecodeBinary(b); e == nil {
		t.Fatal("accepted parent after child")
	}
	if _, e := DecodeBinary(append(bytes.Clone(raw), 0)); e == nil {
		t.Fatal("accepted trailing byte")
	}
	corrupt := bytes.Clone(embedded)
	corrupt[len(corrupt)-8] ^= 1
	if _, e := decodeGzip(corrupt); e == nil {
		t.Fatal("accepted invalid checksum")
	}
}
func TestEmbeddedFailureAlwaysPanics(t *testing.T) {
	l := &Loader{initialErr: io.ErrUnexpectedEOF}
	l.once.Do(func() {})
	for i := 0; i < 2; i++ {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected panic")
				}
			}()
			l.Current()
		}()
	}
}
func TestHTTPTransport(t *testing.T) {
	forced := Handler(nil, HTTPOptions{})
	for _, method := range []string{"GET", "HEAD"} {
		r := httptest.NewRequest(method, "/", nil)
		r.Header.Set("Accept-Encoding", "gzip;q=0")
		w := httptest.NewRecorder()
		forced.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" {
			t.Fatal(w.Code, w.Header())
		}
		if method == "GET" && !bytes.Equal(w.Body.Bytes(), embedded) {
			t.Fatal("not cached bytes")
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD body")
		}
	}
	for _, tag := range []string{DefaultLoader.Current().ETag(), "W/" + DefaultLoader.Current().ETag(), "*"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("If-None-Match", tag)
		w := httptest.NewRecorder()
		forced.ServeHTTP(w, r)
		if w.Code != 304 || w.Body.Len() != 0 {
			t.Fatal("etag", w.Code)
		}
	}
	for _, tc := range []struct {
		header   string
		status   int
		encoding string
	}{
		{"", 200, ""}, {"gzip", 200, "gzip"}, {"gzip;q=0", 200, ""}, {"*;q=0", 406, ""},
		{"gzip;q=0.5, identity;q=0.2", 200, "gzip"}, {"gzip;q=0, identity;q=0", 406, ""},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Accept-Encoding", tc.header)
		w := httptest.NewRecorder()
		Handler(nil, HTTPOptions{DisableForceGzip: true}).ServeHTTP(w, r)
		if w.Code != tc.status || w.Header().Get("Content-Encoding") != tc.encoding {
			t.Fatal(tc, w.Code, w.Header())
		}
		if tc.status == 200 && tc.encoding == "" {
			if _, e := DecodeBinary(w.Body.Bytes()); e != nil {
				t.Fatal(e)
			}
		}
	}
	w := httptest.NewRecorder()
	forced.ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
}
func TestReloadIsolation(t *testing.T) {
	l := &Loader{}
	old := l.Current()
	if e := l.LoadFile("missing.json"); e == nil || l.Current() != old {
		t.Fatal("failed reload changed snapshot")
	}
	b := old.GzipBinary()
	b[0] = 0
	if old.GzipBinary()[0] == 0 {
		t.Fatal("mutable snapshot")
	}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if e := l.LoadFile("data.json"); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if l.Current().Index().Len() == 0 {
					t.Error("empty snapshot")
				}
			}
		}()
	}
	wg.Wait()
}
