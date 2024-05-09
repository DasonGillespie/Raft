package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"time"
)

//---------------// RANDOM STUFF //---------------//

// Helper RPC call messages
type DumpMessage struct {
	Object Node
}

type PingMessage struct {
	String string
}

type StartMessage struct {
	//empty message
}

// Raft rpc call messages and replies
type AppendEntriesMessage struct {
	Term         int
	LeaderID     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []Entry
	LeaderCommit int
	Reconstruct  int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

type RequestVoteMessage struct {
	Term         int
	CandidateID  int
	LastLogIndex int
	LastLogTerm  int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

type CommandMessage struct {
	Command EntryCommand
}

type CommandReply struct {
	Address string
	Success bool
}

// Entries stored in each node's log
type Entry struct {
	Index   int
	Command EntryCommand
	Term    int
}

// The actual commands from the client
type EntryCommand struct {
	Key   string
	Value int
}

// Role types
const (
	Follower = iota
	Candidate
	Leader
)

// The ports the nodes are listening on are predefined
const (
	defaultHost     string = "localhost"
	defaultPort     string = "2200"
	defaultRaftSize int    = 5
	defaultClient   string = "localhost:2200"
	defaultNode1    string = "localhost:2201"
	defaultNode2    string = "localhost:2202"
	defaultNode3    string = "localhost:2203"
	defaultNode4    string = "localhost:2204"
	defaultNode5    string = "localhost:2205"
)

// Network configuration data
// Someimes a node or client will need to loop through them
var RaftNodeAddresses = []string{defaultClient, defaultNode1, defaultNode2, defaultNode3, defaultNode4, defaultNode5}

// Unused at the moment
// Could be used in the future to improve the network configuration
// A node finds its own address
func getLocalAddress() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP.String()
}

//---------------// SERVER //---------------//

func server(nodeID int) {
	fmt.Println("Starting up node server...")
	time.Sleep(time.Second / 5)

	//Initialize node with some default data members
	node := new(Node)
	node.STATE_MACHINE_TABLE = make(map[string]int)
	node.Runnable = false
	node.Address = RaftNodeAddresses[nodeID]
	node.NodeID = nodeID
	node.Role = Follower
	node.ElectionTimeout = rand.Float64()*2 + 1
	node.CurrentTerm = 0
	node.VotedFor = 0
	node.CommitIndex = 0
	node.LastApplied = 0

	//only used by leader
	for i := 0; i < defaultRaftSize; i++ {
		node.NextIndex = append(node.NextIndex, 0)
	}
	for i := 0; i < defaultRaftSize; i++ {
		node.MatchIndex = append(node.MatchIndex, 0)
	}

	//register and listen
	port := strings.TrimPrefix(node.Address, "localhost:")
	rpc.Register(node)
	rpc.HandleHTTP()
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatal("listen error: ", err)
	}
	fmt.Printf("Listening at tcp: %v\n", port)
	fmt.Println()

	//initialize timer
	node.ElectionTimer = time.Now()

	//Implements the "Rules for Servers"
	//as described in the paper
	go RoleRoutines(node)

	if err := http.Serve(listener, nil); err != nil {
		log.Fatalf("http.Serve: %v", err)
	}
}

// Rpc call assistance
func call(address string, method string, request interface{}, response interface{}) error {
	client, err := rpc.DialHTTP("tcp", address)
	if err != nil {
		return err
	}
	defer client.Close()

	if err = client.Call(method, request, response); err != nil {
		log.Printf("Client call to method %s: %v!!!", method, err)
		return err
	}
	return nil
}

func helpStartup() {
	commands := []string{"help", "client", "node", "exit", "quit"}
	fmt.Println()
	fmt.Println("---AVAILABLE COMMANDS---")
	for _, command := range commands {
		fmt.Printf("%s\n", command)
	}
}

func helpClient() {
	commands := []string{"help", "ping", "dump", "exit", "quit"}
	fmt.Println()
	fmt.Println("---AVAILABLE COMMANDS---")
	for _, command := range commands {
		fmt.Printf("%s\n", command)
	}
}

func helpNode() {
	commands := []string{"help", "exit", "quit"}
	fmt.Println()
	fmt.Println("---AVAILABLE COMMANDS---")
	for _, command := range commands {
		fmt.Printf("%s\n", command)
	}
}

func helpDescriptions(role_selected string) {
	fmt.Println()
	switch role_selected {
	case "client":
		fmt.Println("help - list available commands")
		fmt.Println("ping - ping a node")
		fmt.Println("dump - show node data")
		fmt.Println("exit - exit")
		fmt.Println("quit - quit")
	case "node":
	case "":
	default:
	}
}

//---------------// Node //---------------//

type Node struct {
	Runnable        bool
	Address         string //NodeAddress
	LeaderAddress   string
	NodeID          int
	Role            int //Leader, follower, candidate
	ElectionTimer   time.Time
	ElectionTimeout float64

	//persistent state
	CurrentTerm int
	VotedFor    int
	Log         []Entry

	//volitile state
	CommitIndex int
	LastApplied int

	//only used by leaders
	NextIndex  []int
	MatchIndex []int

	//"Backend" store to apply clients commands
	//a real system would have a more sophisticated backend
	STATE_MACHINE_TABLE map[string]int
}

func RoleRoutines(n *Node) {
	for !n.Runnable {
		//client tells node when it is time to start
		//node should not begin routines until all other node are set up
		time.Sleep(time.Second)
	}

	//tracks the nodes that are currently unresponsive
	//used to limit the amount of prints to console
	//only prints the first time a call doesnt work
	out := []int{0, 0, 0, 0, 0}

	for {

		//-----FOLLOWER-----

		//RPC calls will automatically be handled by the server
		for n.Role == Follower {

			//Check Commit Index
			if n.CommitIndex > n.LastApplied {
				command := n.Log[n.LastApplied].Command
				fmt.Printf("\nApplying %s %v to my state table\n", command.Key, command.Value)
				n.STATE_MACHINE_TABLE[command.Key] = command.Value
				n.LastApplied++
			}

			//Check Election Timer
			time_now := time.Now()
			elapsed_time := time_now.Sub(n.ElectionTimer)
			if elapsed_time > time.Duration(int64(float64(time.Second)*n.ElectionTimeout)) {
				//heartbeat not recieved
				//begin election process
				n.Role = Candidate
				fmt.Println("Election timeout expired: Follower -> Candidate")
			}
		}

		//-----CANDIDATE-----

		//Candidate starts the election process by voting for itself
		//If granted the majority of votes -> becomes the leader
		//Otherwise reverts back to follower
		for n.Role == Candidate {
			n.CurrentTerm++
			n.VotedFor = n.NodeID

			//send request votes to all other nodes
			vote_count := 1 //candidate votes for itself
			votes_recieved := 0
			var message RequestVoteMessage
			var reply RequestVoteReply
			message.Term = n.CurrentTerm
			message.CandidateID = n.NodeID
			message.LastLogIndex = len(n.Log) - 1
			if len(n.Log) == 0 {
				message.LastLogTerm = 1
			} else {
				message.LastLogTerm = n.Log[len(n.Log)-1].Term
			}

			for node := 1; node <= 5; node++ {
				//Future work- this might be better to do in go routines
				if node != n.NodeID {
					contact_address := RaftNodeAddresses[node]
					if err := call(contact_address, "Node.RequestVote", &message, &reply); err != nil {
						//log.Printf("Candidate call to _RequestVote_: %v\n", err)
						fmt.Printf("Couldn't contact node %v for RequestVote\n", node)
					} else {
						//Count votes
						//Negative votes are just ignored
						node_term := reply.Term
						votes_recieved++
						if reply.VoteGranted && node_term <= n.CurrentTerm {
							vote_count++
						} else {
							if node_term > n.CurrentTerm {
								n.CurrentTerm = node_term
								n.Role = Follower
								fmt.Println("Discovered node is behind in RequestVote: Candidate -> Follower")
								//stop sending votes
								break
							}
						}
					}
				}
			}
			if vote_count > 2 && n.Role != Follower {
				//got the majority of votes
				//advance term to make sure a new candidate doesnt get ahead before heartbeats
				n.CurrentTerm++
				fmt.Println("\nWon the election: Candidate -> Leader")
				n.Role = Leader
				//reinitialize next index and match index
				for i := 0; i < defaultRaftSize; i++ {
					n.NextIndex[i] = n.LastApplied
				}
				for i := 0; i < defaultRaftSize; i++ {
					n.MatchIndex[i] = 0
				}
			} else {
				//Failed to be elected
				n.VotedFor = 0
				fmt.Println("\nFailed to be elected. Candidate -> Follower")
				n.Role = Follower
				n.ElectionTimer = time.Now()
			}

			//-----LEADER-----

			//leader must continually send out heartbeat messages to other nodes
			//Recieved commands from the client are handled with RPC call "Command"
			for n.Role == Leader {
				//Other nodes set to 0 during appendEntries
				n.VotedFor = 0

				n.LeaderAddress = n.Address
				if n.CommitIndex > n.LastApplied {
					//apply command
					command := n.Log[n.LastApplied].Command
					fmt.Printf("\nApplying %s %v to my state table\n", command.Key, command.Value)
					n.STATE_MACHINE_TABLE[command.Key] = command.Value
					n.LastApplied++
				}

				//The heartbeats can clog up the network if they are running full speed
				//I currently set it to send a heartbeat at least twice per the minimum election timer
				//Future work - send heartbeats on their own timer instead of sleeping
				time.Sleep(time.Second / 2)

				//prep for heartbeats
				var RPCmessage AppendEntriesMessage
				var RPCreply AppendEntriesReply
				RPCmessage.Term = n.CurrentTerm
				RPCmessage.LeaderID = n.NodeID
				RPCmessage.PrevLogIndex = len(n.Log) - 1
				if len(n.Log) == 0 {
					RPCmessage.PrevLogTerm = 0
				} else {
					RPCmessage.PrevLogTerm = n.Log[len(n.Log)-1].Term
				}
				//Should be empty. Nodes will check this early on in AppendEntries to minimize work.
				RPCmessage.Entries = []Entry{}
				RPCmessage.LeaderCommit = n.CommitIndex

				for i := 1; i <= 5; i++ {
					//Future work - this might also be better in a go routine
					if n.NodeID != i {
						contact_address := RaftNodeAddresses[i]
						if err := call(contact_address, "Node.AppendEntries", &RPCmessage, &RPCreply); err != nil {
							if out[i-1] == 0 {
								fmt.Printf("Unable to contact node %v in Heartbeat. Will continue trying.\n", i)
								out[i-1] = 1
							}
							//log.Printf("Leader call to _AppendEntries_ as heartbeat: %v\n", err)
						} else {
							if out[i-1] == 1 {
								//First time the node has responded after comming back up
								out[i-1] = 0
								fmt.Printf("Node %v reconnected\n", i)

								//immediately send log for reconstruction
								RPCmessage.Entries = n.Log
								RPCmessage.Reconstruct = 1
								if err := call(contact_address, "Node.AppendEntries", &RPCmessage, &RPCreply); err != nil {

								} else {
									fmt.Printf("Node %v's log was filled in\n", i)
								}
							} else if !RPCreply.Success || n.CurrentTerm < RPCreply.Term {
								n.Role = Follower
								fmt.Println("Discovered node is behind during heartbeat. Leader -> Follower")
								break
							}
						}
					}
				}
			}
		}
	}
}

func (s *Node) Start(message *StartMessage, reply *StartMessage) error {
	s.Runnable = true
	time.Sleep(time.Second / 2)
	return nil
}

func (s *Node) Command(message *CommandMessage, reply *CommandReply) error {
	if s.Role != Leader {
		//If not the leader, respond with the leader's address
		reply.Address = s.LeaderAddress
		reply.Success = false
		return nil
	}
	reply.Address = ""

	//Append entry to the leader's log
	var entry Entry
	entry.Term = s.CurrentTerm
	entry.Command = message.Command
	entry.Index = len(s.Log)
	s.Log = append(s.Log, entry)
	s.LastApplied++

	//Send entry to all other nodes
	applied_to_log := 0
	var RPCmessage AppendEntriesMessage
	var RPCreply AppendEntriesReply
	RPCmessage.Term = s.CurrentTerm
	RPCmessage.LeaderID = s.NodeID
	RPCmessage.LeaderCommit = s.CommitIndex
	RPCmessage.PrevLogIndex = len(s.Log) - 1
	if len(s.Log) == 0 {
		RPCmessage.PrevLogTerm = 0
	} else {
		RPCmessage.PrevLogTerm = s.Log[len(s.Log)-1].Term
	}

	for i := 1; i <= 5; i++ {
		if i != s.NodeID {
			go func(i int) {
				node_success := false
				for !node_success {
					contact_address := RaftNodeAddresses[i]

					//Determine how many entries to send
					RPCmessage.Entries = []Entry{}
					if s.LastApplied > s.NextIndex[i-1] {
						for entry_index := s.NextIndex[i-1]; entry_index < len(s.Log); entry_index++ {
							RPCmessage.Entries = append(RPCmessage.Entries, s.Log[entry_index])
						}
					}

					if err := call(contact_address, "Node.AppendEntries", &RPCmessage, &RPCreply); err != nil {
						//log.Printf("Leader call to _AppendEntries_ in _Command_: %v\n", err)

						//Do nothing here. If a node doesnt respond the leader will be aware of it through heartbeats.
						//The unresponsive node will be brought up to speed later on
						//Just need at least the majority to respond
					} else {
						node_term := RPCreply.Term
						if node_term > s.CurrentTerm {
							//Node is behind and didnt know it yet. Revert to Follower.
							fmt.Println("Dicover node is behind during Command: Leader -> Follower")
							s.Role = Follower
							reply.Success = false
							break
						}
						node_success = RPCreply.Success
						if node_success {
							applied_to_log++
							s.NextIndex[i-1] = len(s.Log)
							s.MatchIndex[i-1] = len(s.Log) - 1
						} else {
							fmt.Println("Decrementing NextIndex")
							s.NextIndex[i-1]--
						}
					}
				}
			}(i)
		}
	}

	for applied_to_log < 3 {
		//Wait for the majority to apply the entry
		//Future work - better implemented with a channel
	}
	fmt.Println("\nRecieved positive from majority of nodes to apply command to log")
	fmt.Println("Applying command to my state table")

	//Apply command to backend
	s.STATE_MACHINE_TABLE[message.Command.Key] = message.Command.Value
	s.CommitIndex++
	reply.Success = true
	return nil
}

func (s *Node) AppendEntries(message *AppendEntriesMessage, reply *AppendEntriesReply) error {
	//Ensure term isnt behind
	term := message.Term
	reply.Term = s.CurrentTerm
	if term < s.CurrentTerm {
		reply.Success = false
		return nil
	}

	s.ElectionTimer = time.Now()

	//Set role to follower in case of a newly elected leader
	//Only leaders are able to send AppendEntries
	s.Role = Follower
	if s.LeaderAddress != RaftNodeAddresses[message.LeaderID] {
		//print("New Leader Address: " + RaftNodeAddresses[message.LeaderID] + "\n")
	}
	s.LeaderAddress = RaftNodeAddresses[message.LeaderID]
	s.CurrentTerm = term
	s.VotedFor = 0

	if len(message.Entries) != 0 {
		//Check previous log entry
		prevLogTerm := message.PrevLogTerm
		prevLogIndex := message.PrevLogIndex

		//Log might be too short to do a lookup
		if len(s.Log) < prevLogIndex+1 {
			//do nothing
		} else if s.Log[prevLogIndex].Term != prevLogTerm {
			fmt.Println("Replying false in AppendEntries due to term of previous log being different")
			return nil
		}

		if len(s.Log) == 0 && message.Entries[0].Index != 0 {
			fmt.Println("Replying false in AppendEntries due to length of log being 0 and EntryIndex not 0")
			reply.Success = false
			return nil
		}

		//fmt.Printf("Length of message.Entries %v\n", len(message.Entries))
		for i := 0; i < len(message.Entries); i++ {
			index := prevLogIndex + i
			if index < len(s.Log) {
				//Verify own entries
				if s.Log[index].Term != message.Entries[i].Term {
					fmt.Println("Deleting entry and all of it's following")
					s.Log = append(s.Log[:index], s.Log[index:]...)
				}
			} else {
				//Add new entries
				s.Log = append(s.Log, message.Entries[i])
				s.LastApplied = len(s.Log) - 1
				fmt.Println("\nAppending entry to log")
			}
		}
	}

	//Node came back after being unresponsive
	if message.Reconstruct == 1 {
		for i := 0; i < len(s.Log); i++ {
			s.Runnable = true
			s.STATE_MACHINE_TABLE[s.Log[i].Command.Key] = s.Log[i].Command.Value
		}
	}

	//Update commitIndex. This will result in a Node applying an
	//entry to its state if the leader has already done so
	s.CommitIndex = message.LeaderCommit

	reply.Success = true
	return nil
}

func (s *Node) RequestVote(message *RequestVoteMessage, reply *RequestVoteReply) error {
	term := message.Term
	candidateID := message.CandidateID
	lastLogIndex := message.LastLogIndex
	lastLogTerm := message.LastLogTerm

	//Always reply with current term
	reply.Term = s.CurrentTerm

	//Make sure candidate is not behind
	if term < s.CurrentTerm {
		reply.VoteGranted = false
		return nil
	}

	if term > s.CurrentTerm {
		s.CurrentTerm = term
		s.Role = Follower
	}

	//Compare logs
	if s.VotedFor == 0 || s.VotedFor == candidateID {
		var myLastLogTerm int
		if len(s.Log) == 0 {
			myLastLogTerm = 1
		} else {
			myLastLogTerm = s.Log[len(s.Log)-1].Term
		}
		if lastLogTerm > myLastLogTerm {
			if s.Role != Leader {
				//Candidate is more up-to-date
				reply.VoteGranted = true
				s.VotedFor = candidateID
				s.ElectionTimer = time.Now()
				return nil
			}
		} else if lastLogTerm == myLastLogTerm {
			//Last terms match
			//Longer log wins
			if lastLogIndex >= len(s.Log)-1 {
				if s.Role != Leader {
					//Candidate's log is at least as long as reciever's
					reply.VoteGranted = true
					s.VotedFor = candidateID
					s.ElectionTimer = time.Now()
					return nil
				}
			} else {
				//Reciever's log is longer than candidate's
				reply.VoteGranted = false
				return nil
			}
		} else {
			//Receiver is more up-to-date
			reply.VoteGranted = false
			return nil
		}
	} else {
		reply.VoteGranted = false
		return nil
	}

	return nil
}

func (n *Node) Ping(args *PingMessage, response *PingMessage) error {
	pong := "Pong from " + n.Address
	response.String = pong
	fmt.Println("Ping requested")
	fmt.Println()
	return nil
}

func (n *Node) Dump(args *DumpMessage, response *DumpMessage) error {
	//Copies the node and sends it back
	response.Object = *n
	return nil
}

//---------------// MAIN //---------------//

func main() {

	fmt.Println()
	fmt.Print("RAFT INTERFACE")
	fmt.Println()
	fmt.Println()

	helpStartup()
	fmt.Println()

	rand.Seed(time.Now().UnixNano())

	role_selected := ""
	leader_address := defaultNode1

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		parts := strings.SplitN(line, " ", 3)
		if len(parts) > 1 {
			parts[1] = strings.TrimSpace(parts[1])
		}

		if len(parts) == 0 {
			continue
		}

		//main client loop
		switch parts[0] {
		case "start":
			for i := 1; i <= 5; i++ {
				contact_address := RaftNodeAddresses[i]
				var args StartMessage
				var response StartMessage
				go func() {
					if err := call(contact_address, "Node.Start", &args, &response); err != nil {
						log.Printf("Client call to _Start_: %v\n", err)
						fmt.Println("Shutting Down...")
						time.Sleep(time.Second)
						os.Exit(0)
					} else {
						fmt.Println(contact_address + " has started")
					}
				}()
			}
		case "command":
			if len(parts) != 3 {
				fmt.Println("usage: command <key> <value>")
			} else {
				contact_address := leader_address
				safeguard := 0
				var RPCmessage CommandMessage
				var entry EntryCommand
				entry.Key = parts[1]
				val, err := strconv.Atoi(parts[2])
				if err != nil {
					fmt.Println("Value could not be converted to integer")
					continue
				}
				entry.Value = val
				RPCmessage.Command = entry

				complete := false

				for !complete {
					time.Sleep(time.Second)
					var RPCreply CommandReply
					if err := call(contact_address, "Node.Command", &RPCmessage, &RPCreply); err != nil {
						contact_address = RaftNodeAddresses[safeguard]
						safeguard++
						if safeguard > len(RaftNodeAddresses)-1 {
							safeguard = 0
						}
					} else {
						if RPCreply.Address != "" {
							fmt.Printf("Rerouting to Leader: %s\n", RPCreply.Address)
							contact_address = RPCreply.Address
							leader_address = RPCreply.Address
						} else {
							if RPCreply.Success {
								complete = true
								fmt.Println("\nSuccessfully applied command")
							} else {
								fmt.Println("Command failed")
								complete = true
							}
						}
					}
				}
			}
		case "client":
			if role_selected != "" {
				fmt.Println("role has already been selected")
				fmt.Println()
			} else {
				role_selected = "client"
				fmt.Println("changed role to client")
			}
		case "node":
			if role_selected != "" {
				fmt.Println("role has already been selected")
				fmt.Println()
			} else {
				if len(parts) != 2 {
					fmt.Println("usage: node <num>")
				} else {
					nodeID, err := strconv.Atoi(parts[1])
					if err != nil {
						fmt.Println("Error in conversion from string to integer:", err)
						continue
					}
					if nodeID == 0 || nodeID > 5 {
						fmt.Println("node number must be between 1 and 5")
					} else {
						role_selected = "node"
						go server(nodeID)
					}
				}
			}
		case "help":
			if role_selected == "client" {
				helpClient()
				helpDescriptions("client")
			} else if role_selected == "node" {
				helpNode()
				helpDescriptions("node")
			} else {
				helpStartup()
				helpDescriptions("")
			}
		case "quit":
			fmt.Println("\nquiting...")
			fmt.Println()
			time.Sleep(1 * time.Second)
			os.Exit(0)
		case "exit":
			fmt.Println("\nquiting...")
			fmt.Println()
			time.Sleep(1 * time.Second)
			os.Exit(0)
		case "ping":
			if len(parts) != 2 {
				fmt.Println("\nUsage: ping <address>")
				fmt.Println()
			} else {
				if role_selected == "client" {
					contact_address := "localhost:" + parts[1]
					var response PingMessage
					var args PingMessage
					if err := call(contact_address, "Node.Ping", &args, &response); err != nil {
						log.Printf("Client call to _Ping_: %v\n", err)
						fmt.Println("Note : Requested node might not be availble")
						fmt.Println()
					} else {
						fmt.Printf("%s\n", response.String)
					}
				} else {
					fmt.Println("Only clients are allowed to ping")
				}
			}
		case "dump":
			if len(parts) != 2 {
				fmt.Println("\nUsage: dump <port>")
				fmt.Println()
			} else if role_selected == "client" {
				contact_address := "localhost:" + parts[1]
				var response DumpMessage
				var args DumpMessage
				if err := call(contact_address, "Node.Dump", &args, &response); err != nil {
					log.Printf("Client call to _Dump_: %v\n", err)
					fmt.Println("Note : Requested node might not be available")
					fmt.Println()
				} else {
					CP := response.Object
					fmt.Printf("\n\t & NODE %v &\n", CP.NodeID)
					fmt.Println("-----------------------------")
					fmt.Println()
					fmt.Printf("LeaderAddress: %s\n", CP.LeaderAddress)
					var r string
					if CP.Role == 0 {
						r = "FOLLOWER"
					} else if CP.Role == 1 {
						r = "CANDIDATE"
					} else if CP.Role == 2 {
						r = "LEADER"
					}
					fmt.Printf("Role: %s\n", r)
					fmt.Printf("CurrentTerm: %v\n", CP.CurrentTerm)
					fmt.Printf("LastApplied: %v\n", CP.LastApplied)
					fmt.Printf("CommitIndex: %v\n", CP.CommitIndex)
					fmt.Println("\nLOG:")
					fmt.Println("------")
					for i := 0; i < len(CP.Log); i++ {
						fmt.Printf("[%v:%v] %s %v\n", CP.Log[i].Term, CP.Log[i].Index, CP.Log[i].Command.Key, CP.Log[i].Command.Value)
					}
					fmt.Println("\nState:")
					fmt.Println("------")
					for key, value := range CP.STATE_MACHINE_TABLE {
						fmt.Println("Key:", key, "Value:", value)
					}

					//other information here
					fmt.Println("\n-----------------------------")
					fmt.Println()
				}
			} else {
				fmt.Println("A role must first be selected")
				fmt.Println()
			}
		default:
			fmt.Println("\nUnrecogized command")
			fmt.Println("Type 'help' to see available commands")
			fmt.Println()
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("Scanner error: %v", err)
	}
}
