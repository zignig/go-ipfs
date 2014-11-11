package commands

import (
	cmds "github.com/jbenet/go-ipfs/commands"
	config "github.com/jbenet/go-ipfs/config"
)

type VersionOutput struct {
	Version string
}

var versionCmd = &cmds.Command{
	Description: "Outputs the current version of IPFS",
	Help: `Returns the version number of IPFS and exits.
`,

	Options: []cmds.Option{
		cmds.BoolOption("number", "n", "Only output the version number"),
	},
	Run: func(req cmds.Request) (interface{}, error) {
		return &VersionOutput{
			Version: config.CurrentVersionNumber,
		}, nil
	},
	Marshallers: map[cmds.EncodingType]cmds.Marshaller{
		cmds.Text: func(res cmds.Response) ([]byte, error) {
			v := res.Output().(*VersionOutput)
			s := ""

			number, _ := res.Request().Option("number").Bool()

			if !number {
				s += "ipfs version "
			}
			s += v.Version
			return []byte(s), nil
		},
	},
	Type: &VersionOutput{},
}
