package commands

import (
	"fmt"

	mh "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multihash"
	cmds "github.com/jbenet/go-ipfs/commands"
	"github.com/jbenet/go-ipfs/core"
	"github.com/jbenet/go-ipfs/core/commands2/internal"
	dag "github.com/jbenet/go-ipfs/merkledag"
	u "github.com/jbenet/go-ipfs/util"
)

type RefsOutput struct {
	Refs []string
}

var refsCmd = &cmds.Command{
	Description: "Lists link hashes from an object",
	Help: `Retrieves the object named by <ipfs-path> and displays the link
hashes it contains, with the following format:

  <link base58 hash>

Note: list all refs recursively with -r.`,

	Arguments: []cmds.Argument{
		cmds.StringArg("ipfs-path", true, true, "Path to the object(s) to list refs from"),
	},
	Options: []cmds.Option{
		cmds.BoolOption("unique", "u", "Omit duplicate refs from output"),
		cmds.BoolOption("recursive", "r", "Recursively list links of child nodes"),
	},
	Run: func(req cmds.Request) (interface{}, error) {
		n := req.Context().Node

		unique, _ := req.Option("unique").Bool()
		recursive, _ := req.Option("recursive").Bool()

		paths, err := internal.CastToStrings(req.Arguments())
		if err != nil {
			return nil, err
		}

		return getRefs(n, paths, unique, recursive)
	},
	Type: &RefsOutput{},
	Marshallers: map[cmds.EncodingType]cmds.Marshaller{
		cmds.Text: func(res cmds.Response) ([]byte, error) {
			output := res.Output().(*RefsOutput)
			s := ""
			for _, ref := range output.Refs {
				s += fmt.Sprintln(ref)
			}
			return []byte(s), nil
		},
	},
}

func getRefs(n *core.IpfsNode, paths []string, unique, recursive bool) (*RefsOutput, error) {
	var refsSeen map[u.Key]bool
	if unique {
		refsSeen = make(map[u.Key]bool)
	}

	refs := make([]string, 0)

	for _, path := range paths {
		object, err := n.Resolver.ResolvePath(path)
		if err != nil {
			return nil, err
		}

		refs, err = addRefs(n, object, refs, refsSeen, recursive)
		if err != nil {
			return nil, err
		}
	}

	return &RefsOutput{refs}, nil
}

func addRefs(n *core.IpfsNode, object *dag.Node, refs []string, refsSeen map[u.Key]bool, recursive bool) ([]string, error) {
	for _, link := range object.Links {
		var found bool
		found, refs = addRef(link.Hash, refs, refsSeen)

		if recursive && !found {
			child, err := n.DAG.Get(u.Key(link.Hash))
			if err != nil {
				return nil, fmt.Errorf("cannot retrieve %s (%s)", link.Hash.B58String(), err)
			}

			refs, err = addRefs(n, child, refs, refsSeen, recursive)
			if err != nil {
				return nil, err
			}
		}
	}

	return refs, nil
}

func addRef(h mh.Multihash, refs []string, refsSeen map[u.Key]bool) (bool, []string) {
	if refsSeen != nil {
		_, found := refsSeen[u.Key(h)]
		if found {
			return true, refs
		}
		refsSeen[u.Key(h)] = true
	}

	refs = append(refs, h.B58String())
	return false, refs
}
