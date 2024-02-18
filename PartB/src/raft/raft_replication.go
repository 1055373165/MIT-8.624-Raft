package raft

import (
	"sort"
	"time"
)

// 每一个 Entry（槽）参考给出的 ApplyMsg
type LogEntry struct {
	CommandValid bool        // if it should be applied to state machine
	Command      interface{} // the command should be applied to state machine
	Term         int
}

type AppendEntriesArgs struct {
	Term     int
	LeaderId int

	// used to probe the match point （根据论文中的图 2）
	// 一个 index 和 term 可以唯一的确定一个 LogEntry
	/*
	** 1. term 是自增的
	** 2. 算法保证在每一个 term 内，最多只有一个 leader
	** 3. leader 从应用层接收到的日志就可以唯一的确定该 term 中所有的 LogEntry
	 */

	PrevLogIndex int
	PrevLogTerm  int
	// Logs after the match point are appended to the follower's local log entry
	Entries []LogEntry

	// used to update the follower's commitIndex
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

const (
	replicationInterval time.Duration = 200 * time.Millisecond
)

// 四、状态机
func (rf *Raft) becomeFollowerLocked(term int) {
	// 1. 检查当前状态是否允许我们变成 follower
	if term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DError, "Can't become follower, lower term: T%d", term)
		return
	}

	LOG(rf.me, rf.currentTerm, DLog, "%s -> Follower at term: T%v->T%v", rf.role, rf.currentTerm, term)
	rf.role = Follower
	if term > rf.currentTerm {
		rf.votedFor = -1
	}
	rf.currentTerm = term
}

func (rf *Raft) becomeCandidateLocked() {
	if rf.role == Leader {
		LOG(rf.me, rf.currentTerm, DError, "Leader can't become candidator")
		return
	}

	LOG(rf.me, rf.currentTerm, DVote, "%s -> Candidator at term: T%v", rf.role, rf.currentTerm+1)
	rf.currentTerm++
	rf.role = Candidator
	rf.votedFor = rf.me
}

func (rf *Raft) becomeLeaderLocked() {
	if rf.role != Candidator {
		LOG(rf.me, rf.currentTerm, DError, "Only Candidator can become Leader")
		return
	}

	LOG(rf.me, rf.currentTerm, DLeader, "Become Leader at term: T%v", rf.currentTerm)
	rf.role = Leader

	// nextIndex array initialized to leader last log index + 1
	for peer := 0; peer < len(rf.peers); peer++ {
		rf.nextIndex[peer] = len(rf.log)
		rf.matchIndex[peer] = 0
	}
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	// Your code here (PartA).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.currentTerm, rf.role == Leader
}

// Peer's callback(Receive RPC)
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// for debug
	LOG(rf.me, rf.currentTerm, DDebug, "<- S%d, Receive Log, Prev=[%d]T%d, Len()=%d", args.LeaderId, args.PrevLogIndex, args.PrevLogTerm, len(args.Entries))

	// reply initialized for return early
	reply.Term = rf.currentTerm
	reply.Success = false

	// align the term
	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Reject, Higher term, T%d <- T%d", args.LeaderId, args.Term, rf.currentTerm)
		return
	}
	if args.Term >= rf.currentTerm {
		rf.becomeFollowerLocked(args.Term)
	}

	// return failure if the previous log not matched
	if args.PrevLogIndex >= len(rf.log) {
		// DLog2 used for receive RPC handle
		LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Reject Log, Follower log too short, Len: %d <= Prev:%d", args.LeaderId, len(rf.log), args.PrevLogIndex)
		return
	}
	// -> prevLogIndex match

	// return failure if the previous log term not match
	// 1. rf.log[args.PrevLogIndex] is follower local log entry at PrevLogIndex
	// 2. args.PrevLogTerm is leader log entry at PrevLogIndex
	if rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Reject Log, Prev log not match, [%d]: T%d != T%d", args.LeaderId, args.PrevLogIndex, rf.log[args.PrevLogIndex].Term, args.PrevLogTerm)
		return
	}
	// -> prevLogTerm match

	// append the leader logs entries to local(need truncate)
	// rf.log[:args.PrevLogIndex+1] is follower local log entries already match
	// args.Entries need append to rf.log[args.PrevLogIndex+1:]
	rf.log = append(rf.log[:args.PrevLogIndex+1], args.Entries...)
	reply.Success = true
	LOG(rf.me, rf.currentTerm, DLog2, "Follower append logs: (%d, %d]", args.PrevLogIndex, args.PrevLogIndex+len(args.Entries))

	// handle LeaderCommit
	if args.LeaderCommit > rf.commitIndex {
		LOG(rf.me, rf.currentTerm, DApply, "Follower update the commit index %d->%d", rf.commitIndex, args.LeaderCommit)
		rf.commitIndex = args.LeaderCommit
		if rf.commitIndex >= len(rf.log) {
			rf.commitIndex = len(rf.log) - 1
		}
		rf.applyCond.Signal()
	}

	rf.resetElectionTimerLocked()
}

// (send RPC)
func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) replicationTicker(term int) {

	for !rf.killed() {
		ok := rf.startReplication(term)
		if !ok {
			break
		}
		time.Sleep(replicationInterval)
	}
}

func (rf *Raft) getMajorityIndexLocked() int {
	tmpIndexes := make([]int, len(rf.peers))
	copy(tmpIndexes, rf.matchIndex)
	sort.Ints(sort.IntSlice(tmpIndexes))
	// len(peers) is generally odd [(3-1)/2 = 1]
	// Take the median leader matchIndex array, which is the log index value that has been applied to most nodes
	majorityIdx := (len(rf.peers) - 1) / 2
	LOG(rf.me, rf.currentTerm, DDebug, "Match index after sort: %v, majority[%d]=%d", tmpIndexes, majorityIdx, tmpIndexes[majorityIdx])
	return tmpIndexes[majorityIdx]
}

// only valid in the given `term`
func (rf *Raft) startReplication(term int) bool {
	replicationToPeer := func(peer int, args *AppendEntriesArgs) {
		reply := &AppendEntriesReply{}
		ok := rf.sendAppendEntries(peer, args, reply)

		rf.mu.Lock()
		defer rf.mu.Unlock()
		if !ok {
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Lost or crashed", peer)
			return
		}

		// algin term
		if reply.Term > rf.currentTerm {
			rf.becomeFollowerLocked(reply.Term)
			return
		}

		// check context lost
		if rf.contextLostLocked(Leader, term) {
			LOG(rf.me, rf.currentTerm, DLog, "Context Lost, T%d:Leader->T%d:%s", term, rf.currentTerm, rf.role)
			return
		}

		// handle the reply
		// probe the lower index if the prevLog not matched
		if !reply.Success {
			// go back a term(Involves performance optimization)
			// idx where not match, go back to the last log index of the previous term
			idx, term := args.PrevLogIndex, args.PrevLogTerm
			for idx > 0 && rf.log[idx].Term == term {
				idx--
			}
			rf.nextIndex[peer] = idx + 1
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Not matched at %d, try next=%d", peer, args.PrevLogIndex, rf.nextIndex[peer])
			// the timeout also needs to be reset when the logs do not match.
			rf.resetElectionTimerLocked()
			return
		}

		// update match/next index if log appended successfully
		rf.matchIndex[peer] = args.PrevLogIndex + len(args.Entries) // important
		rf.nextIndex[peer] = rf.matchIndex[peer] + 1

		// update the commitIndex
		majorityMatched := rf.getMajorityIndexLocked()
		if majorityMatched > rf.commitIndex {
			LOG(rf.me, rf.currentTerm, DApply, "Leader update the commit index %d->%d", rf.commitIndex, args.LeaderCommit)
			rf.commitIndex = majorityMatched
			rf.applyCond.Signal()
		}
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.contextLostLocked(Leader, term) {
		LOG(rf.me, rf.currentTerm, DLog, "Lost Leader [%d] to %s[T%d]", term, rf.role, rf.currentTerm)
		return false
	}

	// Send AppendEntries RPC to all followers
	for peer := 0; peer < len(rf.peers); peer++ {
		if peer == rf.me {
			rf.matchIndex[peer] = len(rf.log) - 1
			rf.nextIndex[peer] = len(rf.log)
			continue
		}

		// nextIndex is the index of the next log entry to send to the peer
		// matchIndex is the index of the highest log entry known to be replicated on the peer
		// 1. nextIndex is initialized to leader last log index + 1 (nextIndex initialized when become leader)
		// 2. matchIndex is initialized to 0
		// 3. replication to peer will start from nextIndex - 1
		prevIdx := rf.nextIndex[peer] - 1
		prevTerm := rf.log[prevIdx].Term
		args := &AppendEntriesArgs{
			Term:         rf.currentTerm,
			LeaderId:     rf.me,
			PrevLogIndex: prevIdx,
			PrevLogTerm:  prevTerm,
			Entries:      rf.log[prevIdx+1:],
			LeaderCommit: rf.commitIndex, // tell follower the commit index
		}

		go replicationToPeer(peer, args)
	}
	return true
}
