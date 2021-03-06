package ipldzec

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"

	cid "github.com/ipfs/go-cid"
	node "github.com/ipfs/go-ipld-format"
	mh "github.com/multiformats/go-multihash"
)

type Tx struct {
	Version    uint32           `json:"version"`
	Inputs     []*TxIn          `json:"inputs"`
	Outputs    []*TxOut         `json:"outputs"`
	LockTime   uint32           `json:"locktime"`
	JoinSplits []*JSDescription `json:"joinSplits,omitempty"`
	JSPubKey   []byte           `json:"jsPubKey,omitempty"`
	JSSig      []byte           `json:"jsSig,omitempty"`
}

func (t *Tx) Cid() *cid.Cid {
	h, _ := mh.Sum(t.RawData(), mh.DBL_SHA2_256, -1)
	return cid.NewCidV1(cid.ZcashTx, h)
}

func (t *Tx) Links() []*node.Link {
	var out []*node.Link
	for i, input := range t.Inputs {
		if input.PrevTx != nil {
			lnk := &node.Link{Cid: input.PrevTx}
			lnk.Name = fmt.Sprintf("inputs/%d/prevTx", i)
			out = append(out, lnk)
		}
	}
	return out
}

func (t *Tx) RawData() []byte {
	buf := new(bytes.Buffer)
	i := make([]byte, 4)
	binary.LittleEndian.PutUint32(i, t.Version)
	buf.Write(i)
	writeVarInt(buf, uint64(len(t.Inputs)))
	for _, inp := range t.Inputs {
		inp.WriteTo(buf)
	}

	writeVarInt(buf, uint64(len(t.Outputs)))
	for _, out := range t.Outputs {
		out.WriteTo(buf)
	}

	binary.LittleEndian.PutUint32(i, t.LockTime)
	buf.Write(i)
	if t.Version == 1 {
		return buf.Bytes()
	}

	writeVarInt(buf, uint64(len(t.JoinSplits)))
	for _, js := range t.JoinSplits {
		js.WriteTo(buf)
	}

	buf.Write(t.JSPubKey)
	buf.Write(t.JSSig)

	return buf.Bytes()
}

func (t *Tx) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"type": "zcashTx",
	}
}

func (t *Tx) Resolve(path []string) (interface{}, []string, error) {
	switch path[0] {
	case "version":
		return t.Version, path[1:], nil
	case "lockTime":
		return t.LockTime, path[1:], nil
	case "inputs":
		if len(path) == 1 {
			return t.Inputs, nil, nil
		}

		index, err := strconv.Atoi(path[1])
		if err != nil {
			return nil, nil, err
		}

		if index >= len(t.Inputs) || index < 0 {
			return nil, nil, fmt.Errorf("index out of range")
		}

		inp := t.Inputs[index]
		if len(path) == 2 {
			return inp, nil, nil
		}

		switch path[2] {
		case "prevTx":
			if inp.PrevTx == nil {
				return nil, nil, fmt.Errorf("no such link")
			}
			return &node.Link{Cid: inp.PrevTx}, path[3:], nil
		case "seqNo":
			return inp.SeqNo, path[3:], nil
		case "script":
			return inp.Script, path[3:], nil
		default:
			return nil, nil, fmt.Errorf("no such link")
		}
	case "outputs":
		if len(path) == 1 {
			return t.Outputs, nil, nil
		}

		index, err := strconv.Atoi(path[1])
		if err != nil {
			return nil, nil, err
		}

		if index >= len(t.Outputs) || index < 0 {
			return nil, nil, fmt.Errorf("index out of range")
		}

		outp := t.Outputs[index]
		if len(path) == 2 {
			return outp, path[2:], nil
		}

		switch path[2] {
		case "value":
			return outp.Value, path[3:], nil
		case "script":
			return outp.Script, path[3:], nil
		default:
			return nil, nil, fmt.Errorf("no such link")
		}
	case "joinSplits":
		return t.JoinSplits, path[1:], nil
	case "jsPubKey":
		return t.JSPubKey, path[1:], nil
	case "jsSig":
		return t.JSSig, path[1:], nil
	default:
		return nil, nil, fmt.Errorf("no such link")
	}
}

func (t *Tx) ResolveLink(path []string) (*node.Link, []string, error) {
	i, rest, err := t.Resolve(path)
	if err != nil {
		return nil, rest, err
	}

	lnk, ok := i.(*node.Link)
	if !ok {
		return nil, nil, fmt.Errorf("value was not a link")
	}

	return lnk, rest, nil
}

func (t *Tx) Size() (uint64, error) {
	return uint64(len(t.RawData())), nil
}

func (t *Tx) Stat() (*node.NodeStat, error) {
	return &node.NodeStat{}, nil
}

func (t *Tx) Copy() node.Node {
	nt := *t // cheating shallow copy
	return &nt
}

func (t *Tx) String() string {
	return fmt.Sprintf("zcash transaction")
}

func (t *Tx) Tree(p string, depth int) []string {
	if depth == 0 {
		return nil
	}

	switch p {
	case "inputs":
		return t.treeInputs(nil, depth+1)
	case "outputs":
		return t.treeOutputs(nil, depth+1)
	case "":
		out := []string{"version", "timeLock", "inputs", "outputs", "joinSplits", "jsPubKey", "jsSig"}
		out = t.treeInputs(out, depth)
		out = t.treeOutputs(out, depth)
		return out
	default:
		return nil
	}
}

func (t *Tx) treeInputs(out []string, depth int) []string {
	if depth < 2 {
		return out
	}

	for i, _ := range t.Inputs {
		inp := "inputs/" + fmt.Sprint(i)
		out = append(out, inp)
		if depth > 2 {
			out = append(out, inp+"/prevTx", inp+"/seqNo", inp+"/script")
		}
	}
	return out
}

func (t *Tx) treeOutputs(out []string, depth int) []string {
	if depth < 2 {
		return out
	}

	for i, _ := range t.Outputs {
		o := "outputs/" + fmt.Sprint(i)
		out = append(out, o)
		if depth > 2 {
			out = append(out, o+"/script", o+"/value")
		}
	}
	return out
}

func (t *Tx) ZecSha() []byte {
	mh, _ := mh.Sum(t.RawData(), mh.DBL_SHA2_256, -1)
	return []byte(mh[2:])
}

func (t *Tx) HexHash() string {
	return hex.EncodeToString(revString(t.ZecSha()))
}

func txHashToLink(b []byte) *node.Link {
	mhb, _ := mh.Encode(b, mh.DBL_SHA2_256)
	c := cid.NewCidV1(cid.ZcashTx, mhb)
	return &node.Link{Cid: c}
}

type TxIn struct {
	PrevTx      *cid.Cid `json:"txid,omitempty"`
	PrevTxIndex uint32   `json:"vout"`
	Script      []byte   `json:"script"`
	SeqNo       uint32   `json:"sequence"`
}

func (i *TxIn) WriteTo(w io.Writer) error {
	buf := make([]byte, 36)
	if i.PrevTx != nil {
		copy(buf[:32], cidToHash(i.PrevTx))
	}
	binary.LittleEndian.PutUint32(buf[32:36], i.PrevTxIndex)
	w.Write(buf)

	writeVarInt(w, uint64(len(i.Script)))
	w.Write(i.Script)
	binary.LittleEndian.PutUint32(buf[:4], i.SeqNo)
	w.Write(buf[:4])
	return nil
}

type TxOut struct {
	Value  uint64 `json:"value"`
	Script []byte `json:"script"`
}

func (o *TxOut) WriteTo(w io.Writer) error {
	val := make([]byte, 8)
	binary.LittleEndian.PutUint64(val, o.Value)
	w.Write(val)
	writeVarInt(w, uint64(len(o.Script)))
	w.Write(o.Script)
	return nil
}

var _ node.Node = (*Tx)(nil)
