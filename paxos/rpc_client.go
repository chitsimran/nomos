package paxos

import "net/rpc"

func CallPing(addr string, fromID int) bool {
	client, err := rpc.Dial("tcp", addr)
	if err != nil {
		return false
	}
	defer client.Close()

	args := PingArgs{FromID: fromID}
	var reply PingReply

	err = client.Call("Node.HandlePing", &args, &reply)
	if err != nil {
		return false
	}

	return reply.OK
}

func CallRPC[Args any, Reply any](
	addr string,
	method string,
	args *Args,
) (*Reply, bool) {

	client, err := rpc.Dial("tcp", addr)
	if err != nil {
		return nil, false
	}
	defer client.Close()

	var reply Reply
	if err := client.Call(method, args, &reply); err != nil {
		return nil, false
	}

	return &reply, true
}

func CallPrepare(addr string, args *PrepareArgs) (*PrepareReply, bool) {
	return CallRPC[PrepareArgs, PrepareReply](
		addr,
		"Node.HandlePrepare",
		args,
	)
}

func CallAccept(addr string, args *AcceptArgs) (*AcceptReply, bool) {
	return CallRPC[AcceptArgs, AcceptReply](
		addr,
		"Node.HandleAccept",
		args,
	)
}

func CallCommit(addr string, args *CommitArgs) (*CommitReply, bool) {
	return CallRPC[CommitArgs, CommitReply](
		addr,
		"Node.HandleCommit",
		args,
	)
}

func CallHeartbeat(addr string, args *HeartbeatArgs) (*HeartbeatReply, bool) {
	return CallRPC[HeartbeatArgs, HeartbeatReply](
		addr,
		"Node.HandleHeartbeat",
		args,
	)
}
