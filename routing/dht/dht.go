package dht

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	inet "github.com/jbenet/go-ipfs/net"
	msg "github.com/jbenet/go-ipfs/net/message"
	peer "github.com/jbenet/go-ipfs/peer"
	kb "github.com/jbenet/go-ipfs/routing/kbucket"
	u "github.com/jbenet/go-ipfs/util"

	context "github.com/jbenet/go-ipfs/Godeps/_workspace/src/code.google.com/p/go.net/context"
	ds "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-datastore"

	"github.com/jbenet/go-ipfs/Godeps/_workspace/src/code.google.com/p/goprotobuf/proto"
)

var log = u.Logger("dht")

const doPinging = true

// TODO. SEE https://github.com/jbenet/node-ipfs/blob/master/submodules/ipfs-dht/index.js

// IpfsDHT is an implementation of Kademlia with Coral and S/Kademlia modifications.
// It is used to implement the base IpfsRouting module.
type IpfsDHT struct {
	// Array of routing tables for differently distanced nodes
	// NOTE: (currently, only a single table is used)
	routingTables []*kb.RoutingTable

	// the network services we need
	dialer inet.Dialer
	sender inet.Sender

	// Local peer (yourself)
	self peer.Peer

	// Other peers
	peerstore peer.Peerstore

	// Local data
	datastore ds.Datastore
	dslock    sync.Mutex

	providers *ProviderManager

	// When this peer started up
	birth time.Time

	//lock to make diagnostics work better
	diaglock sync.Mutex

	ctx context.Context
}

// NewDHT creates a new DHT object with the given peer as the 'local' host
func NewDHT(ctx context.Context, p peer.Peer, ps peer.Peerstore, dialer inet.Dialer, sender inet.Sender, dstore ds.Datastore) *IpfsDHT {
	dht := new(IpfsDHT)
	dht.dialer = dialer
	dht.sender = sender
	dht.datastore = dstore
	dht.self = p
	dht.peerstore = ps
	dht.ctx = ctx

	dht.providers = NewProviderManager(p.ID())

	dht.routingTables = make([]*kb.RoutingTable, 3)
	dht.routingTables[0] = kb.NewRoutingTable(20, kb.ConvertPeerID(p.ID()), time.Millisecond*1000)
	dht.routingTables[1] = kb.NewRoutingTable(20, kb.ConvertPeerID(p.ID()), time.Millisecond*1000)
	dht.routingTables[2] = kb.NewRoutingTable(20, kb.ConvertPeerID(p.ID()), time.Hour)
	dht.birth = time.Now()

	if doPinging {
		go dht.PingRoutine(time.Second * 10)
	}
	return dht
}

// Connect to a new peer at the given address, ping and add to the routing table
func (dht *IpfsDHT) Connect(ctx context.Context, npeer peer.Peer) (peer.Peer, error) {
	log.Debug("Connect to new peer: %s", npeer)

	// TODO(jbenet,whyrusleeping)
	//
	// Connect should take in a Peer (with ID). In a sense, we shouldn't be
	// allowing connections to random multiaddrs without knowing who we're
	// speaking to (i.e. peer.ID). In terms of moving around simple addresses
	// -- instead of an (ID, Addr) pair -- we can use:
	//
	//   /ip4/10.20.30.40/tcp/1234/ipfs/Qxhxxchxzcncxnzcnxzcxzm
	//
	err := dht.dialer.DialPeer(npeer)
	if err != nil {
		return nil, err
	}

	// Ping new peer to register in their routing table
	// NOTE: this should be done better...
	err = dht.Ping(ctx, npeer)
	if err != nil {
		return nil, fmt.Errorf("failed to ping newly connected peer: %s\n", err)
	}

	dht.Update(npeer)

	return npeer, nil
}

// HandleMessage implements the inet.Handler interface.
func (dht *IpfsDHT) HandleMessage(ctx context.Context, mes msg.NetMessage) msg.NetMessage {

	mData := mes.Data()
	if mData == nil {
		log.Error("Message contained nil data.")
		return nil
	}

	mPeer := mes.Peer()
	if mPeer == nil {
		log.Error("Message contained nil peer.")
		return nil
	}

	// deserialize msg
	pmes := new(Message)
	err := proto.Unmarshal(mData, pmes)
	if err != nil {
		log.Error("Error unmarshaling data")
		return nil
	}

	// update the peer (on valid msgs only)
	dht.Update(mPeer)

	// Print out diagnostic
	log.Debug("[peer: %s] Got message type: '%s' [from = %s]\n",
		dht.self, Message_MessageType_name[int32(pmes.GetType())], mPeer)

	// get handler for this msg type.
	handler := dht.handlerForMsgType(pmes.GetType())
	if handler == nil {
		log.Error("got back nil handler from handlerForMsgType")
		return nil
	}

	// dispatch handler.
	rpmes, err := handler(mPeer, pmes)
	if err != nil {
		log.Error("handle message error: %s", err)
		return nil
	}

	// if nil response, return it before serializing
	if rpmes == nil {
		log.Warning("Got back nil response from request.")
		return nil
	}

	// serialize response msg
	rmes, err := msg.FromObject(mPeer, rpmes)
	if err != nil {
		log.Error("serialze response error: %s", err)
		return nil
	}

	return rmes
}

// sendRequest sends out a request using dht.sender, but also makes sure to
// measure the RTT for latency measurements.
func (dht *IpfsDHT) sendRequest(ctx context.Context, p peer.Peer, pmes *Message) (*Message, error) {

	mes, err := msg.FromObject(p, pmes)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	// Print out diagnostic
	log.Debug("Sent message type: '%s' [to = %s]",
		Message_MessageType_name[int32(pmes.GetType())], p)

	rmes, err := dht.sender.SendRequest(ctx, mes)
	if err != nil {
		return nil, err
	}
	if rmes == nil {
		return nil, errors.New("no response to request")
	}

	rtt := time.Since(start)
	rmes.Peer().SetLatency(rtt)

	rpmes := new(Message)
	if err := proto.Unmarshal(rmes.Data(), rpmes); err != nil {
		return nil, err
	}

	return rpmes, nil
}

// putValueToNetwork stores the given key/value pair at the peer 'p'
func (dht *IpfsDHT) putValueToNetwork(ctx context.Context, p peer.Peer,
	key string, value []byte) error {

	pmes := newMessage(Message_PUT_VALUE, string(key), 0)
	pmes.Value = value
	rpmes, err := dht.sendRequest(ctx, p, pmes)
	if err != nil {
		return err
	}

	if !bytes.Equal(rpmes.Value, pmes.Value) {
		return errors.New("value not put correctly")
	}
	return nil
}

func (dht *IpfsDHT) putProvider(ctx context.Context, p peer.Peer, key string) error {

	pmes := newMessage(Message_ADD_PROVIDER, string(key), 0)

	// add self as the provider
	pmes.ProviderPeers = peersToPBPeers([]peer.Peer{dht.self})

	rpmes, err := dht.sendRequest(ctx, p, pmes)
	if err != nil {
		return err
	}

	log.Debug("%s putProvider: %s for %s", dht.self, p, key)
	if rpmes.GetKey() != pmes.GetKey() {
		return errors.New("provider not added correctly")
	}

	return nil
}

func (dht *IpfsDHT) getValueOrPeers(ctx context.Context, p peer.Peer,
	key u.Key, level int) ([]byte, []peer.Peer, error) {

	pmes, err := dht.getValueSingle(ctx, p, key, level)
	if err != nil {
		return nil, nil, err
	}

	log.Debug("pmes.GetValue() %v", pmes.GetValue())
	if value := pmes.GetValue(); value != nil {
		// Success! We were given the value
		log.Debug("getValueOrPeers: got value")
		return value, nil, nil
	}

	// TODO decide on providers. This probably shouldn't be happening.
	if prv := pmes.GetProviderPeers(); prv != nil && len(prv) > 0 {
		val, err := dht.getFromPeerList(ctx, key, prv, level)
		if err != nil {
			return nil, nil, err
		}
		log.Debug("getValueOrPeers: get from providers")
		return val, nil, nil
	}

	// Perhaps we were given closer peers
	var peers []peer.Peer
	for _, pb := range pmes.GetCloserPeers() {
		pr, err := dht.peerFromInfo(pb)
		if err != nil {
			log.Error("%s", err)
			continue
		}
		peers = append(peers, pr)
	}

	if len(peers) > 0 {
		log.Debug("getValueOrPeers: peers")
		return nil, peers, nil
	}

	log.Warning("getValueOrPeers: u.ErrNotFound")
	return nil, nil, u.ErrNotFound
}

// getValueSingle simply performs the get value RPC with the given parameters
func (dht *IpfsDHT) getValueSingle(ctx context.Context, p peer.Peer,
	key u.Key, level int) (*Message, error) {

	pmes := newMessage(Message_GET_VALUE, string(key), level)
	return dht.sendRequest(ctx, p, pmes)
}

// TODO: Im not certain on this implementation, we get a list of peers/providers
// from someone what do we do with it? Connect to each of them? randomly pick
// one to get the value from? Or just connect to one at a time until we get a
// successful connection and request the value from it?
func (dht *IpfsDHT) getFromPeerList(ctx context.Context, key u.Key,
	peerlist []*Message_Peer, level int) ([]byte, error) {

	for _, pinfo := range peerlist {
		p, err := dht.ensureConnectedToPeer(pinfo)
		if err != nil {
			log.Error("getFromPeers error: %s", err)
			continue
		}

		pmes, err := dht.getValueSingle(ctx, p, key, level)
		if err != nil {
			log.Error("getFromPeers error: %s\n", err)
			continue
		}

		if value := pmes.GetValue(); value != nil {
			// Success! We were given the value
			dht.providers.AddProvider(key, p)
			return value, nil
		}
	}
	return nil, u.ErrNotFound
}

// getLocal attempts to retrieve the value from the datastore
func (dht *IpfsDHT) getLocal(key u.Key) ([]byte, error) {
	dht.dslock.Lock()
	defer dht.dslock.Unlock()
	v, err := dht.datastore.Get(key.DsKey())
	if err != nil {
		return nil, err
	}

	byt, ok := v.([]byte)
	if !ok {
		return nil, errors.New("value stored in datastore not []byte")
	}
	return byt, nil
}

// putLocal stores the key value pair in the datastore
func (dht *IpfsDHT) putLocal(key u.Key, value []byte) error {
	return dht.datastore.Put(key.DsKey(), value)
}

// Update signals to all routingTables to Update their last-seen status
// on the given peer.
func (dht *IpfsDHT) Update(p peer.Peer) {
	log.Debug("updating peer: %s latency = %f\n", p, p.GetLatency().Seconds())
	removedCount := 0
	for _, route := range dht.routingTables {
		removed := route.Update(p)
		// Only close the connection if no tables refer to this peer
		if removed != nil {
			removedCount++
		}
	}

	// Only close the connection if no tables refer to this peer
	// if removedCount == len(dht.routingTables) {
	// 	dht.network.ClosePeer(p)
	// }
	// ACTUALLY, no, let's not just close the connection. it may be connected
	// due to other things. it seems that we just need connection timeouts
	// after some deadline of inactivity.
}

// FindLocal looks for a peer with a given ID connected to this dht and returns the peer and the table it was found in.
func (dht *IpfsDHT) FindLocal(id peer.ID) (peer.Peer, *kb.RoutingTable) {
	for _, table := range dht.routingTables {
		p := table.Find(id)
		if p != nil {
			return p, table
		}
	}
	return nil, nil
}

func (dht *IpfsDHT) findPeerSingle(ctx context.Context, p peer.Peer, id peer.ID, level int) (*Message, error) {
	pmes := newMessage(Message_FIND_NODE, string(id), level)
	return dht.sendRequest(ctx, p, pmes)
}

func (dht *IpfsDHT) printTables() {
	for _, route := range dht.routingTables {
		route.Print()
	}
}

func (dht *IpfsDHT) findProvidersSingle(ctx context.Context, p peer.Peer, key u.Key, level int) (*Message, error) {
	pmes := newMessage(Message_GET_PROVIDERS, string(key), level)
	return dht.sendRequest(ctx, p, pmes)
}

// TODO: Could be done async
func (dht *IpfsDHT) addProviders(key u.Key, peers []*Message_Peer) []peer.Peer {
	var provArr []peer.Peer
	for _, prov := range peers {
		p, err := dht.peerFromInfo(prov)
		if err != nil {
			log.Error("error getting peer from info: %v", err)
			continue
		}

		log.Debug("%s adding provider: %s for %s", dht.self, p, key)

		// Dont add outselves to the list
		if p.ID().Equal(dht.self.ID()) {
			continue
		}

		// TODO(jbenet) ensure providers is idempotent
		dht.providers.AddProvider(key, p)
		provArr = append(provArr, p)
	}
	return provArr
}

// nearestPeersToQuery returns the routing tables closest peers.
func (dht *IpfsDHT) nearestPeersToQuery(pmes *Message, count int) []peer.Peer {
	level := pmes.GetClusterLevel()
	cluster := dht.routingTables[level]

	key := u.Key(pmes.GetKey())
	closer := cluster.NearestPeers(kb.ConvertKey(key), count)
	return closer
}

// betterPeerToQuery returns nearestPeersToQuery, but iff closer than self.
func (dht *IpfsDHT) betterPeersToQuery(pmes *Message, count int) []peer.Peer {
	closer := dht.nearestPeersToQuery(pmes, count)

	// no node? nil
	if closer == nil {
		return nil
	}

	// == to self? thats bad
	for _, p := range closer {
		if p.ID().Equal(dht.self.ID()) {
			log.Error("Attempted to return self! this shouldnt happen...")
			return nil
		}
	}

	var filtered []peer.Peer
	for _, p := range closer {
		// must all be closer than self
		key := u.Key(pmes.GetKey())
		if !kb.Closer(dht.self.ID(), p.ID(), key) {
			filtered = append(filtered, p)
		}
	}

	// ok seems like closer nodes
	return filtered
}

func (dht *IpfsDHT) getPeer(id peer.ID) (peer.Peer, error) {
	p, err := dht.peerstore.Get(id)
	if err != nil {
		err = fmt.Errorf("Failed to get peer from peerstore: %s", err)
		log.Error("%s", err)
		return nil, err
	}
	return p, nil
}

func (dht *IpfsDHT) peerFromInfo(pbp *Message_Peer) (peer.Peer, error) {

	id := peer.ID(pbp.GetId())

	// bail out if it's ourselves
	//TODO(jbenet) not sure this should be an error _here_
	if id.Equal(dht.self.ID()) {
		return nil, errors.New("found self")
	}

	p, err := dht.getPeer(id)
	if err != nil {
		return nil, err
	}

	maddr, err := pbp.Address()
	if err != nil {
		return nil, err
	}
	p.AddAddress(maddr)
	return p, nil
}

func (dht *IpfsDHT) ensureConnectedToPeer(pbp *Message_Peer) (peer.Peer, error) {
	p, err := dht.peerFromInfo(pbp)
	if err != nil {
		return nil, err
	}

	// dial connection
	err = dht.dialer.DialPeer(p)
	return p, err
}

//TODO: this should be smarter about which keys it selects.
func (dht *IpfsDHT) loadProvidableKeys() error {
	kl, err := dht.datastore.KeyList()
	if err != nil {
		return err
	}
	for _, dsk := range kl {
		k := u.KeyFromDsKey(dsk)
		if len(k) == 0 {
			log.Error("loadProvidableKeys error: %v", dsk)
		}

		dht.providers.AddProvider(k, dht.self)
	}
	return nil
}

// PingRoutine periodically pings nearest neighbors.
func (dht *IpfsDHT) PingRoutine(t time.Duration) {
	tick := time.Tick(t)
	for {
		select {
		case <-tick:
			id := make([]byte, 16)
			rand.Read(id)
			peers := dht.routingTables[0].NearestPeers(kb.ConvertKey(u.Key(id)), 5)
			for _, p := range peers {
				ctx, _ := context.WithTimeout(dht.ctx, time.Second*5)
				err := dht.Ping(ctx, p)
				if err != nil {
					log.Error("Ping error: %s", err)
				}
			}
		case <-dht.ctx.Done():
			return
		}
	}
}

// Bootstrap builds up list of peers by requesting random peer IDs
func (dht *IpfsDHT) Bootstrap(ctx context.Context) {
	id := make([]byte, 16)
	rand.Read(id)
	_, err := dht.FindPeer(ctx, peer.ID(id))
	if err != nil {
		log.Error("Bootstrap peer error: %s", err)
	}
}
