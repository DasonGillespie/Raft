# Raft

## Introduction
Raft is a distributed consensus protocol designed to ensure fault-tolerant and reliable operation of a distributed system. It achieves this by establishing a leader among a group of nodes, allowing for coordinated decision-making and replication of data across multiple servers. Raft operates through a series of leader elections, where nodes compete to become the authoritative leader responsible for coordinating the system's operations. Once a leader is established, it manages the replication of data by coordinating with follower nodes, ensuring consistency and fault tolerance even in the event of node failures or network partitions. Raft provides a straightforward and understandable approach to distributed consensus, making it a popular choice for building robust distributed systems.

The Raft project I have created is integrated with a simple text-based application that implements and demonstrates the Raft protocol. Using a testbed of five node, a user can send key-value pairs through the system and verify the replicated data. A user can also simulate node failure and re-entry, testing the protocol's fault tolerance. In the current state this project is in, it does not support log compaction or membership changes.

## Usage
Running this project will require spinning up 6 instances. I recommend using tmux for window management.

Begin by starting up each of the 5 nodes. Run the executable:

```bash
./raft
```

Then identify the node with:

```bash
node <num>
#For nodes 1-5
```

Once all 5 nodes are up, start up the client in the same manner. but instead of giving it a node number just run:

```bash
client
```

At this point, all of the node are just running idle, waiting for the client to tell them to begin running raft. Do this with:

```bash
start
```

All the nodes will begin running raft and should quickly elect a leader.

At any point in time, the client can recieve and display information pertaining to one of the nodes. Do this now with:

```bash
dump 220<node_num>
#Where node_num is any of the numbers 1-5 you specified when starting up the node
#The nodes are set up to run locally on ports 2201-2205
```

You should see the Node's ID, the current leader's address, the node's current role and some other information pertaining to the raft protocol. The node's Log and State should be empty at this time. At all times, the node's logs and states should be the same (the states might be in a different order, but the information within should be correct)

You can issue a command in the form of key-value pairs that all the nodes will store.

```bash
command <key> <value>
```

After issuing a command, dump a few of the nodes and note the changes.

The protocol get really interesting when a node drops out. Simulate this with a keyboard interrupt or some other method on one of the nodes. Then try issuing a command and then bring the node back in. After the node drops out, the remaining nodes should elect a new leader, correctly apply the command, and then get the lost node up to speed once it rejoins.

