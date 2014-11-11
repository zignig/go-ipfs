package commands

import (
	"errors"

	cmds "github.com/jbenet/go-ipfs/commands"
)

var resolveCmd = &cmds.Command{
	Description: "Gets the value currently published at an IPNS name",
	Help: `IPNS is a PKI namespace, where names are the hashes of public keys, and
the private key enables publishing new (signed) values. In resolve, the
default value of <name> is your own identity public key.


Examples:

Resolve the value of your identity:

  > ipfs name resolve
  QmatmE9msSfkKxoffpHwNLNKgwZG8eT9Bud6YoPab52vpy

Resolve te value of another name:

  > ipfs name resolve QmbCMUZw6JFeZ7Wp9jkzbye3Fzp2GGcPgC3nmeUjfVF87n
  QmatmE9msSfkKxoffpHwNLNKgwZG8eT9Bud6YoPab52vpy

`,

	Arguments: []cmds.Argument{
		cmds.StringArg("name", false, false, "The IPNS name to resolve. Defaults to your node's peerID."),
	},
	Run: func(req cmds.Request) (interface{}, error) {

		n := req.Context().Node
		var name string

		if n.Network == nil {
			return nil, errNotOnline
		}

		if len(req.Arguments()) == 0 {
			if n.Identity == nil {
				return nil, errors.New("Identity not loaded!")
			}
			name = n.Identity.ID().String()

		} else {
			var ok bool
			name, ok = req.Arguments()[0].(string)
			if !ok {
				return nil, errors.New("cast error")
			}
		}

		output, err := n.Namesys.Resolve(name)
		if err != nil {
			return nil, err
		}

		return output, nil
	},
	Marshallers: map[cmds.EncodingType]cmds.Marshaller{
		cmds.Text: func(res cmds.Response) ([]byte, error) {
			output := res.Output().(string)
			return []byte(output), nil
		},
	},
}
