package admindivision

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"unicode/utf8"
)

const noParent = ^uint32(0)
const maxCode = uint64(999999999999)

type Node struct {
	Record
	Level      uint8   `json:"level"`
	ParentCode *string `json:"parentCode"`
}

// CompileJSON validates the maintained JSON and emits deterministic gzip binary.
func CompileJSON(data []byte) ([]byte, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("invalid UTF-8 JSON")
	}
	var raw []struct {
		Code       string          `json:"code"`
		Name       string          `json:"name"`
		Pinyin     string          `json:"pinyin"`
		Deprecated *Deprecation    `json:"deprecated"`
		Level      uint8           `json:"level"`
		Parent     json.RawMessage `json:"parentCode"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	nodes := make([]Node, len(raw))
	for i, r := range raw {
		if r.Deprecated == nil || len(r.Parent) == 0 {
			return nil, fmt.Errorf("missing fields for %s", r.Code)
		}
		var parent *string
		if err := json.Unmarshal(r.Parent, &parent); err != nil {
			return nil, err
		}
		nodes[i] = Node{Record{r.Code, r.Name, r.Pinyin, *r.Deprecated}, r.Level, parent}
	}
	ordered, err := orderNodes(nodes)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("CADB")
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint32(len(ordered)))
	positions := make(map[string]uint32, len(ordered))
	for i, n := range ordered {
		code, _ := strconv.ParseUint(n.Code, 10, 64)
		parent := noParent
		if n.ParentCode != nil {
			parent = positions[*n.ParentCode]
		}
		binary.Write(&b, binary.LittleEndian, code)
		binary.Write(&b, binary.LittleEndian, parent)
		b.Write([]byte{n.Level, byte(n.Deprecated), byte(len(n.Code)), 0})
		binary.Write(&b, binary.LittleEndian, uint16(len(n.Name)))
		binary.Write(&b, binary.LittleEndian, uint16(len(n.Pinyin)))
		b.WriteString(n.Name)
		b.WriteString(n.Pinyin)
		positions[n.Code] = uint32(i)
	}
	var compressed bytes.Buffer
	w, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	w.Header.OS = 255
	if _, err = w.Write(b.Bytes()); err != nil {
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

func orderNodes(nodes []Node) ([]Node, error) {
	if len(nodes) == 0 || uint64(len(nodes)) >= uint64(noParent) {
		return nil, fmt.Errorf("invalid record count")
	}
	records := make([]Record, len(nodes))
	byCode := make(map[string]Node, len(nodes))
	for i, n := range nodes {
		records[i] = n.Record
		v, err := strconv.ParseUint(n.Code, 10, 64)
		if err != nil || v > maxCode || len(n.Code) > 12 || n.Level < 1 || n.Level > 4 || !utf8.ValidString(n.Name) || !utf8.ValidString(n.Pinyin) || len(n.Name) > 65535 || len(n.Pinyin) > 65535 {
			return nil, fmt.Errorf("invalid binary fields: %s", n.Code)
		}
		byCode[n.Code] = n
	}
	if _, err := New(records); err != nil {
		return nil, err
	}
	children := make(map[string][]Node)
	for _, n := range nodes {
		parent := ""
		if n.ParentCode == nil {
			if n.Level != 1 {
				return nil, fmt.Errorf("non-province root: %s", n.Code)
			}
		} else {
			p, ok := byCode[*n.ParentCode]
			if !ok || p.Level >= n.Level {
				return nil, fmt.Errorf("invalid parent: %s", n.Code)
			}
			parent = *n.ParentCode
		}
		children[parent] = append(children[parent], n)
	}
	for key := range children {
		sort.Slice(children[key], func(i, j int) bool {
			a, b := children[key][i].Code, children[key][j].Code
			av, _ := strconv.ParseUint(a, 10, 64)
			bv, _ := strconv.ParseUint(b, 10, 64)
			if av == bv {
				return a < b
			}
			return av < bv
		})
	}
	ordered := make([]Node, 0, len(nodes))
	var visit func(string)
	visit = func(parent string) {
		for _, n := range children[parent] {
			ordered = append(ordered, n)
			visit(n.Code)
		}
	}
	visit("")
	if len(ordered) != len(nodes) {
		return nil, fmt.Errorf("unreachable nodes")
	}
	return ordered, nil
}

// DecodeBinary decodes the uncompressed v1 stream and validates all records.
func DecodeBinary(data []byte) ([]Node, error) {
	if len(data) < 10 || string(data[:4]) != "CADB" || binary.LittleEndian.Uint16(data[4:]) != 1 {
		return nil, fmt.Errorf("invalid binary header")
	}
	count := binary.LittleEndian.Uint32(data[6:])
	data = data[10:]
	if count == 0 || uint64(count) > uint64(len(data)/20) {
		return nil, fmt.Errorf("invalid record count")
	}
	nodes := make([]Node, 0, int(count))
	for i := uint32(0); i < count; i++ {
		if len(data) < 20 {
			return nil, io.ErrUnexpectedEOF
		}
		code := binary.LittleEndian.Uint64(data)
		parent := binary.LittleEndian.Uint32(data[8:])
		level, dep, width, reserved := data[12], data[13], int(data[14]), data[15]
		nl, pl := int(binary.LittleEndian.Uint16(data[16:])), int(binary.LittleEndian.Uint16(data[18:]))
		if code > maxCode || width < 1 || width > 12 || len(strconv.FormatUint(code, 10)) > width || reserved != 0 || len(data) < 20+nl+pl {
			return nil, fmt.Errorf("invalid record %d", i)
		}
		text := data[20 : 20+nl+pl]
		if !utf8.Valid(text[:nl]) || !utf8.Valid(text[nl:]) {
			return nil, fmt.Errorf("invalid UTF-8")
		}
		n := Node{Record{fmt.Sprintf("%0*d", width, code), string(text[:nl]), string(text[nl:]), Deprecation(dep)}, level, nil}
		if parent != noParent {
			if parent >= i {
				return nil, fmt.Errorf("parent must precede child")
			}
			p := nodes[parent].Code
			n.ParentCode = &p
		}
		nodes = append(nodes, n)
		data = data[20+nl+pl:]
	}
	if len(data) != 0 {
		return nil, fmt.Errorf("trailing binary data")
	}
	ordered, err := orderNodes(nodes)
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		if nodes[i].Code != ordered[i].Code {
			return nil, fmt.Errorf("noncanonical record order")
		}
	}
	return nodes, nil
}

func decodeGzip(data []byte) ([]Node, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(r)
	closeErr := r.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return DecodeBinary(raw)
}
