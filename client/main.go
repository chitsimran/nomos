package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"nomos/paxos"
	"os"
	"strings"
)

func main() {
	id := flag.Int("id", -1, "unique node ID")
	port := flag.Int("port", -1, "port to listen on")

	flag.Parse()

	if *id < 0 || *port < 0 {
		fmt.Println("Usage: go run cmd/node/main.go --id=<id> --port=<port>")
		os.Exit(1)
	}

	allNodes := map[int]string{
		1: "127.0.0.1:8001",
		2: "127.0.0.1:8002",
		3: "127.0.0.1:8003",
	}

	var peers []string
	for nodeID, addr := range allNodes {
		if nodeID != *id {
			peers = append(peers, addr)
		}
	}

	node := paxos.NewNode(*id, peers)
	wal, _ := paxos.OpenWAL(fmt.Sprintf("log/node-%d.wal", *id))
	node.SetWAL(wal)
	node.LoadFromWAL()

	if err := rpc.Register(node); err != nil {
		log.Fatalf("RPC register failed: %v", err)
	}

	log.SetPrefix(fmt.Sprintf("[Node %d] ", *id))

	go startREPL(*id, node)
	go node.ElectionLoop()
	go node.ApplierLoop()

	listenAddr := fmt.Sprintf(":%d", *port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Listen failed: %v", err)
	}

	log.Printf("Node %d listening on %s\n", *id, listenAddr)
	rpc.Accept(listener)
}

func startREPL(nodeID int, node *paxos.Node) {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Commands:")
	fmt.Println("  ping <addr>")
	fmt.Println("  prepare")
	fmt.Println("  put")
	fmt.Println("  kv")
	fmt.Println("  log")
	fmt.Println("  exit")

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			return
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Split(line, " ")
		switch parts[0] {

		case "ping":
			if len(parts) != 2 {
				fmt.Println("Usage: ping <addr>")
				continue
			}
			addr := parts[1]
			ok := paxos.CallPing(addr, nodeID)
			if ok {
				fmt.Println("Ping OK")
			} else {
				fmt.Println("Ping FAILED")
			}

		case "status":
			fmt.Println(node.Status())

		case "prepare":
			node.RunPrepare()

		case "put":
			if len(parts) != 3 {
				fmt.Println("usage: put <key> <value>")
				continue
			}

			cmd := paxos.Command{
				Key:   parts[1],
				Value: parts[2],
			}

			err := node.RunAccept(cmd)
			if err != nil {
				fmt.Println(err)
			}

		case "kv":
			node.PrintKV()

		case "log":
			node.PrintLog()

		case "exit":
			fmt.Println("Exiting...")
			os.Exit(0)

		default:
			fmt.Println("Unknown command:", parts[0])
		}
	}
}
