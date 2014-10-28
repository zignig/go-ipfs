package main

import (
	"fmt"
	"io"
	"os"
	"runtime/pprof"

	"github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/camlistore/lock"

	cmds "github.com/jbenet/go-ipfs/commands"
	cmdsCli "github.com/jbenet/go-ipfs/commands/cli"
	cmdsHttp "github.com/jbenet/go-ipfs/commands/http"
	"github.com/jbenet/go-ipfs/config"
	"github.com/jbenet/go-ipfs/core/commands"
	u "github.com/jbenet/go-ipfs/util"
)

// log is the command logger
var log = u.Logger("cmd/ipfs")

func main() {
	args := os.Args[1:]
	root := Root

	req, err := cmdsCli.Parse(args, root)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// if the CLI-specific root doesn't contain the command, use the general root
	if len(req.Path()) == 0 {
		root = commands.Root
		req, err = cmdsCli.Parse(args, root)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	cmd, err := root.Get(req.Path())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	options, err := getOptions(req, root)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if debug, found := options.Option("debug"); found && debug.(bool) {
		u.Debug = true

		// if debugging, setup profiling.
		if u.Debug {
			ofi, err := os.Create("cpu.prof")
			if err != nil {
				fmt.Println(err)
				return
			}
			pprof.StartCPUProfile(ofi)
			defer ofi.Close()
			defer pprof.StopCPUProfile()
		}
	}

	var res cmds.Response
	if root == Root {
		res = root.Call(req)

	} else {
		local := true

		configPath, err := getConfigPath(options)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		lockFilePath, err := config.Path(configPath, DaemonLockFile)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if lk, err := lock.Lock(lockFilePath); err != nil {
			local = false
		} else {
			lk.Close()
		}

		if !local {
			res, err = cmdsHttp.Send(req)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

		} else {
			// TODO: spin up node
			res = root.Call(req)
		}
	}

	if res.Error() != nil {
		fmt.Println(res.Error().Error())

		if cmd.Help != "" && res.Error().Code == cmds.ErrClient {
			// TODO: convert from markdown to ANSI terminal format?
			fmt.Println(cmd.Help)
		}

		os.Exit(1)
	}

	_, err = io.Copy(os.Stdout, res)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func getOptions(req cmds.Request, root *cmds.Command) (cmds.Request, error) {
	tempReq := cmds.NewRequest(req.Path(), req.Options(), nil, nil)

	options, err := root.GetOptions(tempReq.Path())
	if err != nil {
		return nil, err
	}

	err = tempReq.ConvertOptions(options)
	if err != nil {
		return nil, err
	}

	return tempReq, nil
}

func getConfigPath(req cmds.Request) (string, error) {
	if opt, found := req.Option("config"); found {
		return opt.(string), nil
	}

	configPath, err := config.PathRoot()
	if err != nil {
		return "", err
	}
	return configPath, nil
}

func getConfig(path string) (*config.Config, error) {
	configFile, err := config.Filename(path)
	if err != nil {
		return nil, err
	}

	return config.Load(configFile)
}
