package core

import (
	"encoding/base64"
	"errors"
	"fmt"

	context "github.com/jbenet/go-ipfs/Godeps/_workspace/src/code.google.com/p/go.net/context"
	b58 "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-base58"
	ds "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-datastore"
	ma "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multiaddr"

	bserv "github.com/jbenet/go-ipfs/blockservice"
	config "github.com/jbenet/go-ipfs/config"
	diag "github.com/jbenet/go-ipfs/diagnostics"
	exchange "github.com/jbenet/go-ipfs/exchange"
	bitswap "github.com/jbenet/go-ipfs/exchange/bitswap"
	merkledag "github.com/jbenet/go-ipfs/merkledag"
	namesys "github.com/jbenet/go-ipfs/namesys"
	inet "github.com/jbenet/go-ipfs/net"
	mux "github.com/jbenet/go-ipfs/net/mux"
	netservice "github.com/jbenet/go-ipfs/net/service"
	path "github.com/jbenet/go-ipfs/path"
	peer "github.com/jbenet/go-ipfs/peer"
	pin "github.com/jbenet/go-ipfs/pin"
	routing "github.com/jbenet/go-ipfs/routing"
	dht "github.com/jbenet/go-ipfs/routing/dht"
	u "github.com/jbenet/go-ipfs/util"
	ctxc "github.com/jbenet/go-ipfs/util/ctxcloser"
)

var log = u.Logger("core")

// IpfsNode is IPFS Core module. It represents an IPFS instance.
type IpfsNode struct {

	// the node's configuration
	Config *config.Config

	// the local node's identity
	Identity peer.Peer

	// storage for other Peer instances
	Peerstore peer.Peerstore

	// the local datastore
	Datastore ds.ThreadSafeDatastore

	// the network message stream
	Network inet.Network

	// the routing system. recommend ipfs-dht
	Routing routing.IpfsRouting

	// the block exchange + strategy (bitswap)
	Exchange exchange.Interface

	// the block service, get/add blocks.
	Blocks *bserv.BlockService

	// the merkle dag service, get/add objects.
	DAG merkledag.DAGService

	// the path resolution system
	Resolver *path.Resolver

	// the name system, resolves paths to hashes
	Namesys namesys.NameSystem

	// the diagnostics service
	Diagnostics *diag.Diagnostics

	// the pinning manager
	Pinning pin.Pinner

	ctxc.ContextCloser

	onlineMode bool // alternatively, offline
}

// NewIpfsNode constructs a new IpfsNode based on the given config.
func NewIpfsNode(cfg *config.Config, online bool) (n *IpfsNode, err error) {
	success := false // flip to true after all sub-system inits succeed
	defer func() {
		if !success && n != nil {
			n.Close()
		}
	}()

	if cfg == nil {
		return nil, fmt.Errorf("configuration required")
	}

	// derive this from a higher context.
	ctx := context.TODO()
	n = &IpfsNode{
		onlineMode:    online,
		Config:        cfg,
		ContextCloser: ctxc.NewContextCloser(ctx, nil),
	}

	// setup datastore.
	if n.Datastore, err = makeDatastore(cfg.Datastore); err != nil {
		return nil, err
	}

	// setup peerstore + local peer identity
	n.Peerstore = peer.NewPeerstore()
	n.Identity, err = initIdentity(n.Config, n.Peerstore, online)
	if err != nil {
		return nil, err
	}

	// setup online services
	if online {

		dhtService := netservice.NewService(ctx, nil)      // nil handler for now, need to patch it
		exchangeService := netservice.NewService(ctx, nil) // nil handler for now, need to patch it
		diagService := netservice.NewService(ctx, nil)     // nil handler for now, need to patch it

		muxMap := &mux.ProtocolMap{
			mux.ProtocolID_Routing:    dhtService,
			mux.ProtocolID_Exchange:   exchangeService,
			mux.ProtocolID_Diagnostic: diagService,
			// add protocol services here.
		}

		// setup the network
		listenAddrs, err := listenAddresses(cfg)
		if err != nil {
			return nil, err
		}

		n.Network, err = inet.NewIpfsNetwork(ctx, listenAddrs, n.Identity, n.Peerstore, muxMap)
		if err != nil {
			return nil, err
		}
		n.AddCloserChild(n.Network)

		// setup diagnostics service
		n.Diagnostics = diag.NewDiagnostics(n.Identity, n.Network, diagService)
		diagService.SetHandler(n.Diagnostics)

		// setup routing service
		dhtRouting := dht.NewDHT(ctx, n.Identity, n.Peerstore, n.Network, dhtService, n.Datastore)
		// TODO(brian): perform this inside NewDHT factory method
		dhtService.SetHandler(dhtRouting) // wire the handler to the service.
		n.Routing = dhtRouting
		n.AddCloserChild(dhtRouting)

		// setup exchange service
		const alwaysSendToPeer = true // use YesManStrategy
		n.Exchange = bitswap.NetMessageSession(ctx, n.Identity, n.Network, exchangeService, n.Routing, n.Datastore, alwaysSendToPeer)
		// ok, this function call is ridiculous o/ consider making it simpler.

		go initConnections(ctx, n.Config, n.Peerstore, dhtRouting)
	}

	// TODO(brian): when offline instantiate the BlockService with a bitswap
	// session that simply doesn't return blocks
	n.Blocks, err = bserv.NewBlockService(n.Datastore, n.Exchange)
	if err != nil {
		return nil, err
	}

	n.DAG = merkledag.NewDAGService(n.Blocks)
	n.Namesys = namesys.NewNameSystem(n.Routing)
	n.Pinning, err = pin.LoadPinner(n.Datastore, n.DAG)
	if err != nil {
		n.Pinning = pin.NewPinner(n.Datastore, n.DAG)
	}
	n.Resolver = &path.Resolver{DAG: n.DAG}

	success = true
	return n, nil
}

func (n *IpfsNode) OnlineMode() bool {
	return n.onlineMode
}

func initIdentity(cfg *config.Config, peers peer.Peerstore, online bool) (peer.Peer, error) {
	if cfg.Identity.PeerID == "" {
		return nil, errors.New("Identity was not set in config (was ipfs init run?)")
	}

	if len(cfg.Identity.PeerID) == 0 {
		return nil, errors.New("No peer ID in config! (was ipfs init run?)")
	}

	// get peer from peerstore (so it is constructed there)
	id := peer.ID(b58.Decode(cfg.Identity.PeerID))
	peer, err := peers.Get(id)
	if err != nil {
		return nil, err
	}

	// when not online, don't need to parse private keys (yet)
	if online {
		skb, err := base64.StdEncoding.DecodeString(cfg.Identity.PrivKey)
		if err != nil {
			return nil, err
		}

		if err := peer.LoadAndVerifyKeyPair(skb); err != nil {
			return nil, err
		}
	}

	return peer, nil
}

func initConnections(ctx context.Context, cfg *config.Config, pstore peer.Peerstore, route *dht.IpfsDHT) {
	for _, p := range cfg.Bootstrap {
		if p.PeerID == "" {
			log.Errorf("error: peer does not include PeerID. %v", p)
		}

		maddr, err := ma.NewMultiaddr(p.Address)
		if err != nil {
			log.Error(err)
			continue
		}

		// setup peer
		npeer, err := pstore.Get(peer.DecodePrettyID(p.PeerID))
		if err != nil {
			log.Errorf("Bootstrapping error: %v", err)
			continue
		}
		npeer.AddAddress(maddr)

		if _, err = route.Connect(ctx, npeer); err != nil {
			log.Errorf("Bootstrapping error: %v", err)
		}
	}
}

func listenAddresses(cfg *config.Config) ([]ma.Multiaddr, error) {
	var listen []ma.Multiaddr

	if len(cfg.Addresses.Swarm) > 0 {
		maddr, err := ma.NewMultiaddr(cfg.Addresses.Swarm)
		if err != nil {
			return nil, fmt.Errorf("Failure to parse config.Addresses.Swarm: %s", cfg.Addresses.Swarm)
		}

		listen = append(listen, maddr)
	}

	return listen, nil
}
