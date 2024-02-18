package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	//	"bytes"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	//	"course/labgob"
	"partA/src/labrpc"
)

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part PartD you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For PartD:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

const (
	ElectionTimeoutMax = time.Millisecond * 300
	ElectionTimeoutMin = time.Millisecond * 150
	HeartbeatInterval  = time.Millisecond * 100
)

func (rf *Raft) isElectionTimeoutLocked() bool {
	//return time.Since(rf.electionStart) >= rf.electionTimeout
	return rf.electionStart.Add(rf.electionTimeout).Before(time.Now())
}

func (rf *Raft) resetElectionTimeoutLocked() {
	rf.electionStart = time.Now()
	randRange := int64(ElectionTimeoutMax - ElectionTimeoutMin)
	rf.electionTimeout = ElectionTimeoutMin + time.Duration(rand.Int63()%randRange)
}

type Role string

const (
	Leader     Role = "Leader"
	Follower   Role = "Follower"
	Candidator Role = "Candidator"
)

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (PartA, PartB, PartC).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	currentTerm int
	role        Role
	votedFor    int

	electionTimeout time.Duration
	electionStart   time.Time
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	// Your code here (PartA).
	return rf.currentTerm, rf.role == Leader
}

func (rf *Raft) becomeFollowerLocked(term int) {
	if term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DError, "can't become follower, lower term: T%d", term)
		return
	}
	LOG(rf.me, rf.currentTerm, DLog, "%s -> Follower at term: T%d->T%d", rf.role, rf.currentTerm, term)
	rf.role = Follower
	// 注意遇到更高的 term 时，需要将自己的投票权清空
	if term > rf.currentTerm {
		rf.votedFor = -1
	}
	// 更新自己所知的最高任期号
	rf.currentTerm = term
}

func (rf *Raft) becomeCandidatorLocked() {
	if rf.role == Leader {
		LOG(rf.me, rf.currentTerm, DError, "Leader Can't become Candidator")
		return
	}
	LOG(rf.me, rf.currentTerm, DVote, "%s -> Candidator at term: T%d", rf.role, rf.currentTerm+1)
	rf.currentTerm++
	rf.role = Candidator
	rf.votedFor = rf.me
}

func (rf *Raft) becomeLeaderLocked() {
	if rf.role != Candidator {
		LOG(rf.me, rf.currentTerm, DError, "Only Candidator can become Leader")
		return
	}

	LOG(rf.me, rf.currentTerm, DLeader, "Leader at term: T%d", rf.currentTerm)
	rf.role = Leader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	// Your code here (PartC).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// raftstate := w.Bytes()
	// rf.persister.Save(raftstate, nil)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (PartC).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (PartD).

}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (PartA, PartB).
	Term       int
	Candidator int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (PartA).
	Term        int
	GrantedVote bool
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (PartA, PartB).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// return RequestVoteReply
	reply.Term = rf.currentTerm

	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DVote, "-> S%d, Reject Vote Request, Higher Term: T%d > T%d", args.Candidator, args.Term, rf.currentTerm, args.Term)
		reply.GrantedVote = false
		return
	}

	if args.Term > rf.currentTerm {
		rf.becomeFollowerLocked(args.Term)
	}

	if rf.votedFor != -1 {
		LOG(rf.me, rf.currentTerm, DVote, "-> S%d, Reject, Already voted to S%d", args.Candidator, rf.votedFor)
		return
	}

	reply.GrantedVote = true
	rf.votedFor = args.Candidator
	rf.resetElectionTimeoutLocked()
	LOG(rf.me, rf.currentTerm, DVote, "-> S%d, Vote Granted", args.Candidator)
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Reject AppendEntries, Higher Term: T%d > T%d", args.LeaderId, args.Term, rf.currentTerm)
		return
	}

	if args.Term > rf.currentTerm {
		rf.becomeFollowerLocked(args.Term)
	}

	reply.Success = true
	rf.resetElectionTimeoutLocked()
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (PartB).

	return index, term, isLeader
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) electionTicker() {
	for !rf.killed() {

		// Your code here (PartA)
		// Check if a leader election should be started.
		// 这里加锁是因为在判断过程中，可能有其他线程修改了role，导致判断错误。
		func() {
			rf.mu.Lock()
			defer rf.mu.Unlock()
			if rf.role != Leader && rf.isElectionTimeoutLocked() {
				rf.becomeCandidatorLocked()
				go rf.startElection(rf.currentTerm)
			}
		}()
		// pause for a random amount of time between 50 and 350
		// milliseconds.
		ms := 50 + (rand.Int63() % 300)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

func (rf *Raft) startElection(term int) bool {
	// 收集投票（通过 RequestVote RPC）
	votes := 0
	// 请求每一个节点投票，向每一个节点发起 RPC 请求，封装一下
	askVoteFromPeer := func(peer int, args *RequestVoteArgs) {
		reply := &RequestVoteReply{}
		// 调用 sendRequestVote RPC
		ok := rf.sendRequestVote(peer, args, reply)

		// 防止 rf 任期号和身份被修改需要加锁
		rf.mu.Lock()
		defer rf.mu.Unlock()

		if !ok {
			LOG(rf.me, rf.currentTerm, DDebug, "Ask Vote From S%d Failed, Lost Or error", peer)
			return
		}

		// 任期对齐
		if reply.Term > rf.currentTerm {
			// 如果收到的投票响应中携带的任期号大于当前任期号，则自动降级为 Follower
			rf.becomeFollowerLocked(reply.Term)
			return
		}

		// 检查上下文（确保投票期间候选人状态正确）
		if rf.CheckContextLocked(Candidator, term) {
			LOG(rf.me, rf.currentTerm, DVote, "Lost Context, abort RequestVoteReply from S%d", peer)
			return
		}

		// 收到有效的投票响应
		if reply.GrantedVote {
			votes++
			// 一旦收集到足够多的投票，切换为 leader 并开启发起心跳
			if votes > len(rf.peers)/2 {
				rf.becomeLeaderLocked()
				LOG(rf.me, rf.currentTerm, DLeader, "S%d Become Leader at term: T%d", rf.me, rf.currentTerm)
				go rf.replicationTicker(term)
			}
		}
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()
	// 如果当前节点已经是 candidator 或者 已经有其他节点正在竞选，则不再发起选举
	if rf.CheckContextLocked(Candidator, term) {
		LOG(rf.me, rf.currentTerm, DVote, "Lost Candidator to %s, abort RequestVote", rf.role)
		return false
	}

	for peer := 0; peer < len(rf.peers); peer++ {
		if peer == rf.me {
			votes++
			continue
		}

		args := &RequestVoteArgs{
			Term:       term,
			Candidator: rf.me,
		}

		// 异步发起投票请求
		go askVoteFromPeer(peer, args)
	}

	return true
}

func (rf *Raft) CheckContextLocked(role Role, term int) bool {
	return !(rf.role == role && rf.currentTerm == term)
}

func (rf *Raft) replicationTicker(term int) {
	// 不停地发送心跳包，以维持 leader 状态
	for !rf.killed() {
		ok := rf.startReplication(term)
		if !ok {
			break
		}
		time.Sleep(HeartbeatInterval)
	}
}

type AppendEntriesArgs struct {
	Term     int
	LeaderId int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

func (rf *Raft) startReplication(term int) bool {
	replicationToPeer := func(peer int, args *AppendEntriesArgs) {
		reply := &AppendEntriesReply{}
		ok := rf.sendAppendEntries(peer, args, reply)

		rf.mu.Lock()
		defer rf.mu.Unlock()
		if !ok {
			LOG(rf.me, rf.currentTerm, DLog, "appendEntries -> S%d Failed, Lost Or error", peer)
			return
		}

		if reply.Term > rf.currentTerm {
			rf.becomeFollowerLocked(reply.Term)
			return
		}
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.CheckContextLocked(Leader, term) {
		LOG(rf.me, rf.currentTerm, DLog, "Lost Leader [T%d] to %s[T%d]", term, rf.role, rf.currentTerm)
		return false
	}

	for peer := 0; peer < len(rf.peers); peer++ {
		if peer == rf.me {
			continue
		}

		args := &AppendEntriesArgs{
			Term:     term,
			LeaderId: rf.me,
		}

		go replicationToPeer(peer, args)
	}
	return true
}

func (rf *Raft) sendAppendEntries(peer int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[peer].Call("Raft.AppendEntries", args, reply)
	return ok
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (PartA, PartB, PartC).
	rf.currentTerm = 0
	rf.role = Follower
	rf.votedFor = -1

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.electionTicker()

	return rf
}
