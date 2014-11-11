package commands

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/jbenet/go-ipfs/Godeps/_workspace/src/code.google.com/p/go.net/context"

	mh "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multihash"
	"github.com/jbenet/go-ipfs/blocks"
	cmds "github.com/jbenet/go-ipfs/commands"
	u "github.com/jbenet/go-ipfs/util"
)

type Block struct {
	Key    string
	Length int
}

var blockCmd = &cmds.Command{
	Description: "Manipulate raw IPFS blocks",
	Help: `'ipfs block' is a plumbing command used to manipulate raw ipfs blocks.
Reads from stdin or writes to stdout, and <key> is a base58 encoded
multihash.`,
	Subcommands: map[string]*cmds.Command{
		"get": blockGetCmd,
		"put": blockPutCmd,
	},
}

var blockGetCmd = &cmds.Command{
	Description: "Get a raw IPFS block",
	Help: `ipfs block get is a plumbing command for retreiving raw ipfs blocks.
It outputs to stdout, and <key> is a base58 encoded multihash.`,

	Arguments: []cmds.Argument{
		cmds.StringArg("key", true, false, "The base58 multihash of an existing block to get"),
	},
	Run: func(req cmds.Request) (interface{}, error) {
		n := req.Context().Node

		key, ok := req.Arguments()[0].(string)
		if !ok {
			return nil, errors.New("cast error")
		}

		if !u.IsValidHash(key) {
			return nil, cmds.Error{"Not a valid hash", cmds.ErrClient}
		}

		h, err := mh.FromB58String(key)
		if err != nil {
			return nil, err
		}

		k := u.Key(h)
		ctx, _ := context.WithTimeout(context.TODO(), time.Second*5)
		b, err := n.Blocks.GetBlock(ctx, k)
		if err != nil {
			return nil, err
		}

		return bytes.NewReader(b.Data), nil
	},
}

var blockPutCmd = &cmds.Command{
	Description: "Stores input as an IPFS block",
	Help: `ipfs block put is a plumbing command for storing raw ipfs blocks.
It reads from stdin, and <key> is a base58 encoded multihash.`,

	Arguments: []cmds.Argument{
		cmds.FileArg("data", true, false, "The data to be stored as an IPFS block"),
	},
	Run: func(req cmds.Request) (interface{}, error) {
		n := req.Context().Node

		in, ok := req.Arguments()[0].(io.Reader)
		if !ok {
			return nil, errors.New("cast error")
		}

		data, err := ioutil.ReadAll(in)
		if err != nil {
			return nil, err
		}

		b := blocks.NewBlock(data)
		log.Debugf("BlockPut key: '%q'", b.Key())

		k, err := n.Blocks.AddBlock(b)
		if err != nil {
			return nil, err
		}

		return &Block{
			Key:    k.String(),
			Length: len(data),
		}, nil
	},
	Type: &Block{},
	Marshallers: map[cmds.EncodingType]cmds.Marshaller{
		cmds.Text: func(res cmds.Response) ([]byte, error) {
			block := res.Output().(*Block)
			s := fmt.Sprintf("Block added (%v bytes): %s\n", block.Length, block.Key)
			return []byte(s), nil
		},
	},
}
