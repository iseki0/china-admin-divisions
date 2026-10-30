package admindivision

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"sync"
	"testing"
)

func TestEmbedded(t *testing.T) {
	source, err := os.ReadFile("data.json")
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileJSON(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(compiled, embedded) {
		t.Fatal("embedded data is stale; run go generate")
	}
	idx := Default()
	if idx.Len() < 42000 {
		t.Fatalf("unexpected size: %d", idx.Len())
	}
	for _, q := range []string{"北京", "BJS", "běi jīng shì", "110000"} {
		got, err := idx.Search(q, Options{})
		if err != nil || len(got) == 0 || got[0].Code != "110000" {
			t.Fatalf("%q: %v %v", q, got, err)
		}
	}
	if r, ok := idx.Lookup("133100"); !ok || r.Name != "雄安新区" {
		t.Fatalf("Xiongan: %+v %v", r, ok)
	}
	if _, ok := idx.Lookup("unknown"); ok {
		t.Fatal("unknown code found")
	}
	for _, limit := range []int{201, math.MaxInt} {
		got, err := idx.Search("东", Options{Limit: limit})
		if err != nil || len(got) <= 200 || len(got) > limit {
			t.Fatalf("limit %d: count=%d err=%v", limit, len(got), err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := Default()
			if other != idx {
				t.Error("default not shared")
			}
			_, _ = other.Search("东", Options{Limit: 5})
		}()
	}
	wg.Wait()
}

func TestRankingAndIsolation(t *testing.T) {
	records := []Record{
		{"4", "吕镇", "lv zhen", Deprecated},
		{"3", "吕镇", "lv zhen", Suspected},
		{"2", "吕镇", "lv zhen", Unmarked},
		{"1", "吕镇新区", "lv zhen xin qu", Unmarked},
	}
	idx, err := New(records)
	if err != nil {
		t.Fatal(err)
	}
	records[0].Name = "mutated"
	for _, q := range []string{"吕镇", "LǙ ZHÈN", "lu: zhen", "lu\u0308 zhen", "lz"} {
		got, err := idx.Search(q, Options{})
		if err != nil || len(got) != 4 {
			t.Fatalf("%q: %v %v", q, got, err)
		}
		for i, code := range []string{"2", "3", "4", "1"} {
			if got[i].Code != code {
				t.Fatalf("%q: %v", q, got)
			}
		}
		got[0].Name = "mutated"
	}
	got, _ := idx.Search("吕", Options{Limit: 1, CodePrefix: "4"})
	if len(got) != 1 || got[0].Name != "吕镇" {
		t.Fatal(got)
	}
	for _, opts := range []Options{{Limit: -1}, {CodePrefix: "a"}} {
		if _, err := idx.Search("", opts); err == nil {
			t.Fatal("accepted invalid options")
		}
	}
	empty, err := idx.Search(" \t", Options{})
	if err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
	if _, err := New([]Record{records[0], records[0]}); err == nil {
		t.Fatal("accepted duplicate")
	}
	if _, err := New([]Record{{Code: "1", Name: "x", Pinyin: "123"}}); err == nil {
		t.Fatal("accepted invalid pinyin")
	}
}

func TestDeprecationJSON(t *testing.T) {
	for _, input := range []string{"false", `"suspected"`, "true"} {
		var d Deprecation
		if err := json.Unmarshal([]byte(input), &d); err != nil {
			t.Fatal(err)
		}
		got, err := json.Marshal(d)
		if err != nil || string(got) != input {
			t.Fatal(string(got), err)
		}
	}
	for _, input := range []string{"null", "0", `"true"`} {
		var d Deprecation
		if json.Unmarshal([]byte(input), &d) == nil {
			t.Fatal("accepted", input)
		}
	}
}
