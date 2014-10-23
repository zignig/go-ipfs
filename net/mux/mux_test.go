package mux

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	mh "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multihash"
	msg "github.com/jbenet/go-ipfs/net/message"
	pb "github.com/jbenet/go-ipfs/net/mux/internal/pb"
	peer "github.com/jbenet/go-ipfs/peer"

	context "github.com/jbenet/go-ipfs/Godeps/_workspace/src/code.google.com/p/go.net/context"
)

type TestProtocol struct {
	*msg.Pipe
}

func (t *TestProtocol) GetPipe() *msg.Pipe {
	return t.Pipe
}

func newPeer(t *testing.T, id string) peer.Peer {
	mh, err := mh.FromHexString(id)
	if err != nil {
		t.Error(err)
		return nil
	}

	return peer.WithID(peer.ID(mh))
}

func testMsg(t *testing.T, m msg.NetMessage, data []byte) {
	if !bytes.Equal(data, m.Data()) {
		t.Errorf("Data does not match: %v != %v", data, m.Data())
	}
}

func testWrappedMsg(t *testing.T, m msg.NetMessage, pid pb.ProtocolID, data []byte) {
	data2, pid2, err := unwrapData(m.Data())
	if err != nil {
		t.Error(err)
	}

	if pid != pid2 {
		t.Errorf("ProtocolIDs do not match: %v != %v", pid, pid2)
	}

	if !bytes.Equal(data, data2) {
		t.Errorf("Data does not match: %v != %v", data, data2)
	}
}

func TestSimpleMuxer(t *testing.T) {

	// setup
	p1 := &TestProtocol{Pipe: msg.NewPipe(10)}
	p2 := &TestProtocol{Pipe: msg.NewPipe(10)}
	pid1 := pb.ProtocolID_Test
	pid2 := pb.ProtocolID_Routing
	mux1 := NewMuxer(ProtocolMap{
		pid1: p1,
		pid2: p2,
	})
	peer1 := newPeer(t, "11140beec7b5ea3f0fdbc95d0dd47f3c5bc275aaaaaa")
	// peer2 := newPeer(t, "11140beec7b5ea3f0fdbc95d0dd47f3c5bc275bbbbbb")

	// run muxer
	ctx := context.Background()
	mux1.Start(ctx)

	// test outgoing p1
	for _, s := range []string{"foo", "bar", "baz"} {
		p1.Outgoing <- msg.New(peer1, []byte(s))
		testWrappedMsg(t, <-mux1.Outgoing, pid1, []byte(s))
	}

	// test incoming p1
	for _, s := range []string{"foo", "bar", "baz"} {
		d, err := wrapData([]byte(s), pid1)
		if err != nil {
			t.Error(err)
		}
		mux1.Incoming <- msg.New(peer1, d)
		testMsg(t, <-p1.Incoming, []byte(s))
	}

	// test outgoing p2
	for _, s := range []string{"foo", "bar", "baz"} {
		p2.Outgoing <- msg.New(peer1, []byte(s))
		testWrappedMsg(t, <-mux1.Outgoing, pid2, []byte(s))
	}

	// test incoming p2
	for _, s := range []string{"foo", "bar", "baz"} {
		d, err := wrapData([]byte(s), pid2)
		if err != nil {
			t.Error(err)
		}
		mux1.Incoming <- msg.New(peer1, d)
		testMsg(t, <-p2.Incoming, []byte(s))
	}
}

func TestSimultMuxer(t *testing.T) {

	// setup
	p1 := &TestProtocol{Pipe: msg.NewPipe(10)}
	p2 := &TestProtocol{Pipe: msg.NewPipe(10)}
	pid1 := pb.ProtocolID_Test
	pid2 := pb.ProtocolID_Identify
	mux1 := NewMuxer(ProtocolMap{
		pid1: p1,
		pid2: p2,
	})
	peer1 := newPeer(t, "11140beec7b5ea3f0fdbc95d0dd47f3c5bc275aaaaaa")
	// peer2 := newPeer(t, "11140beec7b5ea3f0fdbc95d0dd47f3c5bc275bbbbbb")

	// run muxer
	ctx, cancel := context.WithCancel(context.Background())
	mux1.Start(ctx)

	// counts
	total := 10000
	speed := time.Microsecond * 1
	counts := [2][2][2]int{}

	// run producers at every end sending incrementing messages
	produceOut := func(pid pb.ProtocolID, size int) {
		limiter := time.Tick(speed)
		for i := 0; i < size; i++ {
			<-limiter
			s := fmt.Sprintf("proto %v out %v", pid, i)
			m := msg.New(peer1, []byte(s))
			mux1.Protocols[pid].GetPipe().Outgoing <- m
			counts[pid][0][0]++
			// log.Debug("sent %v", s)
		}
	}

	produceIn := func(pid pb.ProtocolID, size int) {
		limiter := time.Tick(speed)
		for i := 0; i < size; i++ {
			<-limiter
			s := fmt.Sprintf("proto %v in %v", pid, i)
			d, err := wrapData([]byte(s), pid)
			if err != nil {
				t.Error(err)
			}

			m := msg.New(peer1, d)
			mux1.Incoming <- m
			counts[pid][1][0]++
			// log.Debug("sent %v", s)
		}
	}

	consumeOut := func() {
		for {
			select {
			case m := <-mux1.Outgoing:
				data, pid, err := unwrapData(m.Data())
				if err != nil {
					t.Error(err)
				}

				// log.Debug("got %v", string(data))
				_ = data
				counts[pid][1][1]++

			case <-ctx.Done():
				return
			}
		}
	}

	consumeIn := func(pid pb.ProtocolID) {
		for {
			select {
			case m := <-mux1.Protocols[pid].GetPipe().Incoming:
				counts[pid][0][1]++
				// log.Debug("got %v", string(m.Data()))
				_ = m
			case <-ctx.Done():
				return
			}
		}
	}

	go produceOut(pid1, total)
	go produceOut(pid2, total)
	go produceIn(pid1, total)
	go produceIn(pid2, total)
	go consumeOut()
	go consumeIn(pid1)
	go consumeIn(pid2)

	limiter := time.Tick(speed)
	for {
		<-limiter
		got := counts[0][0][0] + counts[0][0][1] +
			counts[0][1][0] + counts[0][1][1] +
			counts[1][0][0] + counts[1][0][1] +
			counts[1][1][0] + counts[1][1][1]

		if got == total*8 {
			cancel()
			return
		}
	}

}

func TestStopping(t *testing.T) {

	// setup
	p1 := &TestProtocol{Pipe: msg.NewPipe(10)}
	p2 := &TestProtocol{Pipe: msg.NewPipe(10)}
	pid1 := pb.ProtocolID_Test
	pid2 := pb.ProtocolID_Identify
	mux1 := NewMuxer(ProtocolMap{
		pid1: p1,
		pid2: p2,
	})
	peer1 := newPeer(t, "11140beec7b5ea3f0fdbc95d0dd47f3c5bc275aaaaaa")
	// peer2 := newPeer(t, "11140beec7b5ea3f0fdbc95d0dd47f3c5bc275bbbbbb")

	// run muxer
	mux1.Start(context.Background())

	// test outgoing p1
	for _, s := range []string{"foo1", "bar1", "baz1"} {
		p1.Outgoing <- msg.New(peer1, []byte(s))
		testWrappedMsg(t, <-mux1.Outgoing, pid1, []byte(s))
	}

	// test incoming p1
	for _, s := range []string{"foo2", "bar2", "baz2"} {
		d, err := wrapData([]byte(s), pid1)
		if err != nil {
			t.Error(err)
		}
		mux1.Incoming <- msg.New(peer1, d)
		testMsg(t, <-p1.Incoming, []byte(s))
	}

	mux1.Stop()
	if mux1.cancel != nil {
		t.Error("mux.cancel should be nil")
	}

	// test outgoing p1
	for _, s := range []string{"foo3", "bar3", "baz3"} {
		p1.Outgoing <- msg.New(peer1, []byte(s))
		select {
		case m := <-mux1.Outgoing:
			t.Errorf("should not have received anything. Got: %v", string(m.Data()))
		case <-time.After(time.Millisecond):
		}
	}

	// test incoming p1
	for _, s := range []string{"foo4", "bar4", "baz4"} {
		d, err := wrapData([]byte(s), pid1)
		if err != nil {
			t.Error(err)
		}
		mux1.Incoming <- msg.New(peer1, d)
		select {
		case <-p1.Incoming:
			t.Error("should not have received anything.")
		case <-time.After(time.Millisecond):
		}
	}

}
