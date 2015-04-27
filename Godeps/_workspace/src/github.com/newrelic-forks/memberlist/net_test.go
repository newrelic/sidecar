package memberlist

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/hashicorp/go-msgpack/codec"
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestHandleCompoundPing(t *testing.T) {
	m := GetMemberlist(t)
	m.config.EnableCompression = false
	defer m.Shutdown()

	var udp *net.UDPConn
	for port := 60000; port < 61000; port++ {
		udpAddr := fmt.Sprintf("127.0.0.1:%d", port)
		udpLn, err := net.ListenPacket("udp", udpAddr)
		if err == nil {
			udp = udpLn.(*net.UDPConn)
			break
		}
	}

	if udp == nil {
		t.Fatalf("no udp listener")
	}

	// Encode a ping
	ping := ping{SeqNo: 42}
	buf, err := encode(pingMsg, ping)
	if err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	// Make a compound message
	compound := makeCompoundMessage([][]byte{buf.Bytes(), buf.Bytes(), buf.Bytes()})

	// Send compound version
	addr := &net.UDPAddr{IP: net.ParseIP(m.config.BindAddr), Port: m.config.BindPort}
	udp.WriteTo(compound.Bytes(), addr)

	// Wait for responses
	go func() {
		time.Sleep(time.Second)
		panic("timeout")
	}()

	for i := 0; i < 3; i++ {
		in := make([]byte, 1500)
		n, _, err := udp.ReadFrom(in)
		if err != nil {
			t.Fatalf("unexpected err %s", err)
		}
		in = in[0:n]

		msgType := messageType(in[0])
		if msgType != ackRespMsg {
			t.Fatalf("bad response %v", in)
		}

		var ack ackResp
		if err := decode(in[1:], &ack); err != nil {
			t.Fatalf("unexpected err %s", err)
		}

		if ack.SeqNo != 42 {
			t.Fatalf("bad sequence no")
		}
	}
}

func TestHandlePing(t *testing.T) {
	m := GetMemberlist(t)
	m.config.EnableCompression = false
	defer m.Shutdown()

	var udp *net.UDPConn
	for port := 60000; port < 61000; port++ {
		udpAddr := fmt.Sprintf("127.0.0.1:%d", port)
		udpLn, err := net.ListenPacket("udp", udpAddr)
		if err == nil {
			udp = udpLn.(*net.UDPConn)
			break
		}
	}

	if udp == nil {
		t.Fatalf("no udp listener")
	}

	// Encode a ping
	ping := ping{SeqNo: 42}
	buf, err := encode(pingMsg, ping)
	if err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	// Send
	addr := &net.UDPAddr{IP: net.ParseIP(m.config.BindAddr), Port: m.config.BindPort}
	udp.WriteTo(buf.Bytes(), addr)

	// Wait for response
	go func() {
		time.Sleep(time.Second)
		panic("timeout")
	}()

	in := make([]byte, 1500)
	n, _, err := udp.ReadFrom(in)
	if err != nil {
		t.Fatalf("unexpected err %s", err)
	}
	in = in[0:n]

	msgType := messageType(in[0])
	if msgType != ackRespMsg {
		t.Fatalf("bad response %v", in)
	}

	var ack ackResp
	if err := decode(in[1:], &ack); err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	if ack.SeqNo != 42 {
		t.Fatalf("bad sequence no")
	}
}

func TestHandlePing_WrongNode(t *testing.T) {
	m := GetMemberlist(t)
	m.config.EnableCompression = false
	defer m.Shutdown()

	var udp *net.UDPConn
	for port := 60000; port < 61000; port++ {
		udpAddr := fmt.Sprintf("127.0.0.1:%d", port)
		udpLn, err := net.ListenPacket("udp", udpAddr)
		if err == nil {
			udp = udpLn.(*net.UDPConn)
			break
		}
	}

	if udp == nil {
		t.Fatalf("no udp listener")
	}

	// Encode a ping, wrong node!
	ping := ping{SeqNo: 42, Node: m.config.Name + "-bad"}
	buf, err := encode(pingMsg, ping)
	if err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	// Send
	addr := &net.UDPAddr{IP: net.ParseIP(m.config.BindAddr), Port: m.config.BindPort}
	udp.WriteTo(buf.Bytes(), addr)

	// Wait for response
	udp.SetDeadline(time.Now().Add(50 * time.Millisecond))
	in := make([]byte, 1500)
	_, _, err = udp.ReadFrom(in)

	// Should get an i/o timeout
	if err == nil {
		t.Fatalf("expected err %s", err)
	}
}

func TestHandleIndirectPing(t *testing.T) {
	m := GetMemberlist(t)
	m.config.EnableCompression = false
	defer m.Shutdown()

	var udp *net.UDPConn
	for port := 60000; port < 61000; port++ {
		udpAddr := fmt.Sprintf("127.0.0.1:%d", port)
		udpLn, err := net.ListenPacket("udp", udpAddr)
		if err == nil {
			udp = udpLn.(*net.UDPConn)
			break
		}
	}

	if udp == nil {
		t.Fatalf("no udp listener")
	}

	// Encode an indirect ping
	ind := indirectPingReq{
		SeqNo:  100,
		Target: net.ParseIP(m.config.BindAddr),
		Port:   uint16(m.config.BindPort),
	}
	buf, err := encode(indirectPingMsg, &ind)
	if err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	// Send
	addr := &net.UDPAddr{IP: net.ParseIP(m.config.BindAddr), Port: m.config.BindPort}
	udp.WriteTo(buf.Bytes(), addr)

	// Wait for response
	go func() {
		time.Sleep(time.Second)
		panic("timeout")
	}()

	in := make([]byte, 1500)
	n, _, err := udp.ReadFrom(in)
	if err != nil {
		t.Fatalf("unexpected err %s", err)
	}
	in = in[0:n]

	msgType := messageType(in[0])
	if msgType != ackRespMsg {
		t.Fatalf("bad response %v", in)
	}

	var ack ackResp
	if err := decode(in[1:], &ack); err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	if ack.SeqNo != 100 {
		t.Fatalf("bad sequence no")
	}
}

func TestTCPPushPull(t *testing.T) {
	m := GetMemberlist(t)
	defer m.Shutdown()
	m.nodes = append(m.nodes, &nodeState{
		Node: Node{
			Name: "Test 0",
			Addr: net.ParseIP(m.config.BindAddr),
			Port: uint16(m.config.BindPort),
		},
		Incarnation: 0,
		State:       stateSuspect,
		StateChange: time.Now().Add(-1 * time.Second),
	})

	addr := fmt.Sprintf("%s:%d", m.config.BindAddr, m.config.BindPort)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("unexpected err %s", err)
	}
	defer conn.Close()

	localNodes := make([]pushNodeState, 3)
	localNodes[0].Name = "Test 0"
	localNodes[0].Addr = net.ParseIP(m.config.BindAddr)
	localNodes[0].Port = uint16(m.config.BindPort)
	localNodes[0].Incarnation = 1
	localNodes[0].State = stateAlive
	localNodes[1].Name = "Test 1"
	localNodes[1].Addr = net.ParseIP(m.config.BindAddr)
	localNodes[1].Port = uint16(m.config.BindPort)
	localNodes[1].Incarnation = 1
	localNodes[1].State = stateAlive
	localNodes[2].Name = "Test 2"
	localNodes[2].Addr = net.ParseIP(m.config.BindAddr)
	localNodes[2].Port = uint16(m.config.BindPort)
	localNodes[2].Incarnation = 1
	localNodes[2].State = stateAlive

	// Send our node state
	header := pushPullHeader{Nodes: 3, ClusterName: m.config.ClusterName}
	hd := codec.MsgpackHandle{}
	enc := codec.NewEncoder(conn, &hd)

	// Send the push/pull indicator
	conn.Write([]byte{byte(pushPullMsg)})

	if err := enc.Encode(&header); err != nil {
		t.Fatalf("unexpected err %s", err)
	}
	for i := 0; i < header.Nodes; i++ {
		if err := enc.Encode(&localNodes[i]); err != nil {
			t.Fatalf("unexpected err %s", err)
		}
	}

	// Read the message type
	var msgType messageType
	if err := binary.Read(conn, binary.BigEndian, &msgType); err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	var bufConn io.Reader = conn
	msghd := codec.MsgpackHandle{}
	dec := codec.NewDecoder(bufConn, &msghd)

	// Check if we have a compressed message
	if msgType == compressMsg {
		var c compress
		if err := dec.Decode(&c); err != nil {
			t.Fatalf("unexpected err %s", err)
		}
		decomp, err := decompressBuffer(&c)
		if err != nil {
			t.Fatalf("unexpected err %s", err)
		}

		// Reset the message type
		msgType = messageType(decomp[0])

		// Create a new bufConn
		bufConn = bytes.NewReader(decomp[1:])

		// Create a new decoder
		dec = codec.NewDecoder(bufConn, &hd)
	}

	// Quit if not push/pull
	if msgType != pushPullMsg {
		t.Fatalf("bad message type")
	}

	if err := dec.Decode(&header); err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	// Allocate space for the transfer
	remoteNodes := make([]pushNodeState, header.Nodes)

	// Try to decode all the states
	for i := 0; i < header.Nodes; i++ {
		if err := dec.Decode(&remoteNodes[i]); err != nil {
			t.Fatalf("unexpected err %s", err)
		}
	}

	if len(remoteNodes) != 1 {
		t.Fatalf("bad response")
	}

	n := &remoteNodes[0]
	if n.Name != "Test 0" {
		t.Fatalf("bad name")
	}
	if bytes.Compare(n.Addr, net.ParseIP(m.config.BindAddr)) != 0 {
		t.Fatal("bad addr")
	}
	if n.Incarnation != 0 {
		t.Fatal("bad incarnation")
	}
	if n.State != stateSuspect {
		t.Fatal("bad state")
	}
}

func TestSendMsg_Piggyback(t *testing.T) {
	m := GetMemberlist(t)
	defer m.Shutdown()

	// Add a message to be broadcast
	a := alive{
		Incarnation: 10,
		Node:        "rand",
		ClusterName: m.config.ClusterName,
		Addr:        []byte{127, 0, 0, 255},
		Meta:        nil,
	}
	m.encodeAndBroadcast("rand", aliveMsg, &a)

	var udp *net.UDPConn
	for port := 60000; port < 61000; port++ {
		udpAddr := fmt.Sprintf("127.0.0.1:%d", port)
		udpLn, err := net.ListenPacket("udp", udpAddr)
		if err == nil {
			udp = udpLn.(*net.UDPConn)
			break
		}
	}

	// Encode a ping
	ping := ping{SeqNo: 42}
	buf, err := encode(pingMsg, ping)
	if err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	// Send
	addr := &net.UDPAddr{IP: net.ParseIP(m.config.BindAddr), Port: m.config.BindPort}
	udp.WriteTo(buf.Bytes(), addr)

	// Wait for response
	go func() {
		time.Sleep(time.Second)
		panic("timeout")
	}()

	in := make([]byte, 1500)
	n, _, err := udp.ReadFrom(in)
	if err != nil {
		t.Fatalf("unexpected err %s", err)
	}
	in = in[0:n]

	msgType := messageType(in[0])
	if msgType != compoundMsg {
		t.Fatalf("bad response %v", in)
	}

	// get the parts
	trunc, parts, err := decodeCompoundMessage(in[1:])
	if trunc != 0 {
		t.Fatalf("unexpected truncation")
	}
	if len(parts) != 2 {
		t.Fatalf("unexpected parts %v", parts)
	}
	if err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	var ack ackResp
	if err := decode(parts[0][1:], &ack); err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	if ack.SeqNo != 42 {
		t.Fatalf("bad sequence no")
	}

	var aliveout alive
	if err := decode(parts[1][1:], &aliveout); err != nil {
		t.Fatalf("unexpected err %s", err)
	}

	if aliveout.Node != "rand" || aliveout.Incarnation != 10 {
		t.Fatalf("bad mesg")
	}
}

func TestEncryptDecryptState(t *testing.T) {
	state := []byte("this is our internal state...")
	config := &Config{
		SecretKey:       []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		ProtocolVersion: ProtocolVersionMax,
	}

	m, err := Create(config)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer m.Shutdown()

	crypt, err := m.encryptLocalState(state)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Create reader, seek past the type byte
	buf := bytes.NewReader(crypt)
	buf.Seek(1, 0)

	plain, err := m.decryptRemoteState(buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if !reflect.DeepEqual(state, plain) {
		t.Fatalf("Decrypt failed: %v", plain)
	}
}

func TestIsSameCluster(t *testing.T) {
	m := GetMemberlist(t)
	defer m.Shutdown()

	m.config.ClusterName = "default"
	clusterName := "default"

	if !m.isSameCluster(clusterName) {
		t.Fatalf("Cluster names should match. %v <-> %v", m.config.ClusterName, clusterName)
	}

	clusterName = "badCluster"

	if m.isSameCluster(clusterName) {
		t.Fatalf("Cluster names should not match. %v <-> %v", m.config.ClusterName, clusterName)
	}
}

// handleSuspect() should discard messages from nodes with different Cluster names
func TestHandleSuspect(t *testing.T) {
	addr1 := getBindAddr()
	addr2 := getBindAddr()
	addr3 := getBindAddr()
	ip1 := []byte(addr1)
	ip2 := []byte(addr2)
	ip3 := []byte(addr3)

	m1 := HostMemberlist(addr1.String(), t, nil)

	// dummy address
	addr := m1.udpListener.LocalAddr()

	a1 := alive{Node: "node1", ClusterName: m1.config.ClusterName, Addr: ip1, Port: 7946, Incarnation: 1}
	a2 := alive{Node: "node2", ClusterName: m1.config.ClusterName, Addr: ip2, Port: 7946, Incarnation: 2}
	a3 := alive{Node: "node3", ClusterName: m1.config.ClusterName, Addr: ip3, Port: 7946, Incarnation: 3}

	a1buf, a1err := encode(aliveMsg, a1)
	if a1err != nil {
		t.Fatal("Unexpected error: %v", a1err)
	}

	a2buf, a2err := encode(aliveMsg, a2)
	if a2err != nil {
		t.Fatal("Unexpected error: %v", a2err)
	}

	a3buf, a3err := encode(aliveMsg, a3)
	if a3err != nil {
		t.Fatal("Unexpected error: %v", a3err)
	}

	m1.handleAlive(a1buf.Bytes()[1:], addr)
	m1.handleAlive(a2buf.Bytes()[1:], addr)
	m1.handleAlive(a3buf.Bytes()[1:], addr)

	if len(m1.nodes) != 3 {
		t.Fatalf("Should have 3 nodes in memberlist, but have %v", len(m1.nodes))
	}

	// All nodes should be in alive state
	for k, v := range m1.nodeMap {
		if v.State != stateAlive {
			t.Fatalf("Node %v should be in alive state", k)
		}
	}

	// Suspect message with same cluster name
	s1 := suspect{Node: "node2", Incarnation: 2, ClusterName: m1.config.ClusterName}

	s1buf, s1err := encode(suspectMsg, s1)
	if s1err != nil {
		t.Fatal("Unexpected error: %v", s1err)
	}

	// Send the message, shouldn't get discarded; state should change
	m1.handleSuspect(s1buf.Bytes()[1:], addr)

	if m1.nodeMap[a2.Node].State != stateSuspect {
		t.Fatal("Node 2 should be in suspect state")
	}

	// Send a suspect message for node 3, but with different cluster name
	s2 := suspect{Node: "node3", Incarnation: 3, ClusterName: "badCluster"}

	s2buf, s2err := encode(suspectMsg, s2)
	if s2err != nil {
		t.Fatal("Unexpected error: %v", s2err)
	}

	m1.handleSuspect(s2buf.Bytes()[1:], addr)

	// Suspect message should be discarded and have not affected 'real' node 3
	if m1.nodeMap[a3.Node].State != stateAlive {
		t.Fatalf("Node 3 should still be in alive state")
	}
}

func TestHandleAlive(t *testing.T) {
	addr1 := getBindAddr()
	addr2 := getBindAddr()
	addr3 := getBindAddr()
	ip1 := []byte(addr1)
	ip2 := []byte(addr2)
	ip3 := []byte(addr3)

	m1 := HostMemberlist(addr1.String(), t, nil)

	// dummy address
	addr := m1.udpListener.LocalAddr()

	a1 := alive{Node: "node1", ClusterName: m1.config.ClusterName, Addr: ip1, Port: 7946, Incarnation: 1}
	a2 := alive{Node: "node2", ClusterName: m1.config.ClusterName, Addr: ip2, Port: 7946, Incarnation: 2}
	a3 := alive{Node: "node3", ClusterName: "badCluster", Addr: ip3, Port: 7946, Incarnation: 2}

	// Encode 2 alive messages with the same cluster name
	a1buf, err := encode(aliveMsg, a1)
	if err != nil {
		t.Fatal("Unexpected error: %v", err)
	}

	a2buf, err2 := encode(aliveMsg, a2)
	if err2 != nil {
		t.Fatal("Unexpected error: %v", err2)
	}

	// Encode a third message with a different cluster name
	a3buf, err3 := encode(aliveMsg, a3)
	if err3 != nil {
		t.Fatal("Unexpected error: %v", err3)
	}

	m1.handleAlive(a1buf.Bytes()[1:], addr)
	m1.handleAlive(a2buf.Bytes()[1:], addr)

	if len(m1.nodes) != 2 {
		t.Fatalf("Should have 2 nodes in memberlist, but have %v", len(m1.nodes))
	}

	// Send 3rd message, which should get ignored
	m1.handleAlive(a3buf.Bytes()[1:], addr)

	if len(m1.nodes) != 2 {
		t.Fatalf("Should still have 2 nodes in memberlist, but have %v", len(m1.nodes))
	}
}

func TestHandleDead(t *testing.T) {
	addr1 := getBindAddr()
	addr2 := getBindAddr()
	addr3 := getBindAddr()
	ip1 := []byte(addr1)
	ip2 := []byte(addr2)
	ip3 := []byte(addr3)

	m1 := HostMemberlist(addr1.String(), t, nil)

	// dummy address
	addr := m1.udpListener.LocalAddr()

	a1 := alive{Node: "node1", ClusterName: m1.config.ClusterName, Addr: ip1, Port: 7946, Incarnation: 1}
	a2 := alive{Node: "node2", ClusterName: m1.config.ClusterName, Addr: ip2, Port: 7946, Incarnation: 2}
	a3 := alive{Node: "node3", ClusterName: m1.config.ClusterName, Addr: ip3, Port: 7946, Incarnation: 3}

	a1buf, a1err := encode(aliveMsg, a1)
	if a1err != nil {
		t.Fatal("Unexpected error: %v", a1err)
	}

	a2buf, a2err := encode(aliveMsg, a2)
	if a2err != nil {
		t.Fatal("Unexpected error: %v", a2err)
	}

	a3buf, a3err := encode(aliveMsg, a3)
	if a3err != nil {
		t.Fatal("Unexpected error: %v", a3err)
	}

	m1.handleAlive(a1buf.Bytes()[1:], addr)
	m1.handleAlive(a2buf.Bytes()[1:], addr)
	m1.handleAlive(a3buf.Bytes()[1:], addr)

	if len(m1.nodes) != 3 {
		t.Fatalf("Should have 3 nodes in memberlist, but have %v", len(m1.nodes))
	}

	// All nodes should be in alive state
	for k, v := range m1.nodeMap {
		if v.State != stateAlive {
			t.Fatalf("Node %v should be in alive state", k)
		}
	}

	// Dead message with same cluster name
	d1 := dead{Node: "node2", ClusterName: m1.config.ClusterName, Incarnation: 2}

	d1buf, d1err := encode(deadMsg, d1)
	if d1err != nil {
		t.Fatal("Unexpected error: %v", d1err)
	}

	// Send the message, shouldn't get discarded; state should change
	m1.handleDead(d1buf.Bytes()[1:], addr)

	if m1.nodeMap[a2.Node].State != stateDead {
		t.Fatal("Node 2 should be in dead state")
	}

	// Send a dead message for node 3, but with different cluster name
	d2 := dead{Node: "node3", ClusterName: "badCluster", Incarnation: 3}

	d2buf, d2err := encode(deadMsg, d2)
	if d2err != nil {
		t.Fatal("Unexpected error: %v", d2err)
	}

	m1.handleDead(d2buf.Bytes()[1:], addr)

	// Dead message should be discarded and have not affected 'real' node 3
	if m1.nodeMap[a3.Node].State != stateAlive {
		t.Fatalf("Node 3 should still be in alive state")
	}
}

// TODO: This probably should get tested as well, but will require a bit of setup
func TestReadRemoteState(t *testing.T) {
}
