package http

import (
	"io"

	core "github.com/jbenet/go-ipfs/core"
	"github.com/jbenet/go-ipfs/importer"
	dag "github.com/jbenet/go-ipfs/merkledag"
	uio "github.com/jbenet/go-ipfs/unixfs/io"
	u "github.com/jbenet/go-ipfs/util"
)

type ipfs interface {
	ResolvePath(string) (*dag.Node, error)
	ResolveName(string) (string, error)
	NewDagFromReader(io.Reader) (*dag.Node, error)
	AddNodeToDAG(nd *dag.Node) (u.Key, error)
	NewDagReader(nd *dag.Node) (io.Reader, error)
}

type ipfsHandler struct {
	node *core.IpfsNode
}

func (i *ipfsHandler) ResolvePath(path string) (*dag.Node, error) {
	return i.node.Resolver.ResolvePath(path)
}

func (i *ipfsHandler) ResolveName(path string) (string, error) {
	return i.node.Namesys.Resolve(path)
}

func (i *ipfsHandler) NewDagFromReader(r io.Reader) (*dag.Node, error) {
	return importer.NewDagFromReader(r)
}

func (i *ipfsHandler) AddNodeToDAG(nd *dag.Node) (u.Key, error) {
	return i.node.DAG.Add(nd)
}

func (i *ipfsHandler) NewDagReader(nd *dag.Node) (io.Reader, error) {
	return uio.NewDagReader(nd, i.node.DAG)
}
