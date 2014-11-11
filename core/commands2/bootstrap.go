package commands

import (
	"errors"
	"fmt"
	"strings"

	ma "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multiaddr"
	mh "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multihash"

	cmds "github.com/jbenet/go-ipfs/commands"
	config "github.com/jbenet/go-ipfs/config"
)

type BootstrapOutput struct {
	Peers []*config.BootstrapPeer
}

var peerOptionDesc = "A peer to add to the bootstrap list (in the format '<multiaddr>/<peerID>')"

var bootstrapCmd = &cmds.Command{
	Description: "Show or edit the list of bootstrap peers",
	Help: `Running 'ipfs bootstrap' with no arguments will run 'ipfs bootstrap list'.
` + bootstrapSecurityWarning,

	Run:         bootstrapListCmd.Run,
	Marshallers: bootstrapListCmd.Marshallers,
	Subcommands: map[string]*cmds.Command{
		"list":   bootstrapListCmd,
		"add":    bootstrapAddCmd,
		"remove": bootstrapRemoveCmd,
	},
}

var bootstrapAddCmd = &cmds.Command{
	Description: "Add peers to the bootstrap list",
	Help: `Outputs a list of peers that were added (that weren't already
in the bootstrap list).
` + bootstrapSecurityWarning,

	Arguments: []cmds.Argument{
		cmds.StringArg("peer", true, true, peerOptionDesc),
	},
	Run: func(req cmds.Request) (interface{}, error) {
		input, err := bootstrapInputToPeers(req.Arguments())
		if err != nil {
			return nil, err
		}

		filename, err := config.Filename(req.Context().ConfigRoot)
		if err != nil {
			return nil, err
		}

		added, err := bootstrapAdd(filename, req.Context().Config, input)
		if err != nil {
			return nil, err
		}

		return &BootstrapOutput{added}, nil
	},
	Type: &BootstrapOutput{},
	Marshallers: map[cmds.EncodingType]cmds.Marshaller{
		cmds.Text: func(res cmds.Response) ([]byte, error) {
			v := res.Output().(*BootstrapOutput)
			s := fmt.Sprintf("Added %v peers to the bootstrap list:\n", len(v.Peers))
			marshalled, err := bootstrapMarshaller(res)
			if err != nil {
				return nil, err
			}
			return append([]byte(s), marshalled...), nil
		},
	},
}

var bootstrapRemoveCmd = &cmds.Command{
	Description: "Removes peers from the bootstrap list",
	Help: `Outputs the list of peers that were removed.
` + bootstrapSecurityWarning,

	Arguments: []cmds.Argument{
		cmds.StringArg("peer", true, true, peerOptionDesc),
	},
	Run: func(req cmds.Request) (interface{}, error) {
		input, err := bootstrapInputToPeers(req.Arguments())
		if err != nil {
			return nil, err
		}

		filename, err := config.Filename(req.Context().ConfigRoot)
		if err != nil {
			return nil, err
		}

		removed, err := bootstrapRemove(filename, req.Context().Config, input)
		if err != nil {
			return nil, err
		}

		return &BootstrapOutput{removed}, nil
	},
	Type: &BootstrapOutput{},
	Marshallers: map[cmds.EncodingType]cmds.Marshaller{
		cmds.Text: func(res cmds.Response) ([]byte, error) {
			v := res.Output().(*BootstrapOutput)
			s := fmt.Sprintf("Removed %v peers from the bootstrap list:\n", len(v.Peers))
			marshalled, err := bootstrapMarshaller(res)
			if err != nil {
				return nil, err
			}
			return append([]byte(s), marshalled...), nil
		},
	},
}

var bootstrapListCmd = &cmds.Command{
	Description: "Show peers in the bootstrap list",
	Help: `Peers are output in the format '<multiaddr>/<peerID>'.
`,

	Run: func(req cmds.Request) (interface{}, error) {
		peers := req.Context().Config.Bootstrap
		return &BootstrapOutput{peers}, nil
	},
	Type: &BootstrapOutput{},
	Marshallers: map[cmds.EncodingType]cmds.Marshaller{
		cmds.Text: bootstrapMarshaller,
	},
}

func bootstrapMarshaller(res cmds.Response) ([]byte, error) {
	v := res.Output().(*BootstrapOutput)

	s := ""
	for _, peer := range v.Peers {
		s += fmt.Sprintf("%s/%s\n", peer.Address, peer.PeerID)
	}

	return []byte(s), nil
}

func bootstrapInputToPeers(input []interface{}) ([]*config.BootstrapPeer, error) {
	split := func(addr string) (string, string) {
		idx := strings.LastIndex(addr, "/")
		if idx == -1 {
			return "", addr
		}
		return addr[:idx], addr[idx+1:]
	}

	peers := []*config.BootstrapPeer{}
	for _, v := range input {
		addr, ok := v.(string)
		if !ok {
			return nil, errors.New("cast error")
		}

		addrS, peeridS := split(addr)

		// make sure addrS parses as a multiaddr.
		if len(addrS) > 0 {
			maddr, err := ma.NewMultiaddr(addrS)
			if err != nil {
				return nil, err
			}

			addrS = maddr.String()
		}

		// make sure idS parses as a peer.ID
		_, err := mh.FromB58String(peeridS)
		if err != nil {
			return nil, err
		}

		// construct config entry
		peers = append(peers, &config.BootstrapPeer{
			Address: addrS,
			PeerID:  peeridS,
		})
	}
	return peers, nil
}

func bootstrapAdd(filename string, cfg *config.Config, peers []*config.BootstrapPeer) ([]*config.BootstrapPeer, error) {
	added := make([]*config.BootstrapPeer, 0, len(peers))

	for _, peer := range peers {
		duplicate := false
		for _, peer2 := range cfg.Bootstrap {
			if peer.Address == peer2.Address {
				duplicate = true
				break
			}
		}

		if !duplicate {
			cfg.Bootstrap = append(cfg.Bootstrap, peer)
			added = append(added, peer)
		}
	}

	err := config.WriteConfigFile(filename, cfg)
	if err != nil {
		return nil, err
	}

	return added, nil
}

func bootstrapRemove(filename string, cfg *config.Config, peers []*config.BootstrapPeer) ([]*config.BootstrapPeer, error) {
	removed := make([]*config.BootstrapPeer, 0, len(peers))
	keep := make([]*config.BootstrapPeer, 0, len(cfg.Bootstrap))

	for _, peer := range cfg.Bootstrap {
		found := false
		for _, peer2 := range peers {
			if peer.Address == peer2.Address && peer.PeerID == peer2.PeerID {
				found = true
				removed = append(removed, peer)
				break
			}
		}

		if !found {
			keep = append(keep, peer)
		}
	}
	cfg.Bootstrap = keep

	err := config.WriteConfigFile(filename, cfg)
	if err != nil {
		return nil, err
	}

	return removed, nil
}

const bootstrapSecurityWarning = `
SECURITY WARNING:

The bootstrap command manipulates the "bootstrap list", which contains
the addresses of bootstrap nodes. These are the *trusted peers* from
which to learn about other peers in the network. Only edit this list
if you understand the risks of adding or removing nodes from this list.

`
