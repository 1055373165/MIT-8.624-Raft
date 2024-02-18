package raft

import (
	"math/rand"
	"time"
)

const (
	electionTimeoutMin time.Duration = 250 * time.Millisecond
	electionTimeoutMax time.Duration = 400 * time.Millisecond
)

// resetElectionTimerLocked resets the election timer when receiving a heartbeat
func (rf *Raft) resetElectionTimerLocked() {
	rf.electionStart = time.Now()
	randRange := int64(electionTimeoutMax - electionTimeoutMin)
	rf.electionTimeout = time.Duration(rand.Int63()%randRange) + electionTimeoutMin
}

// isElectionTimeoutLocked returns true if the election timer has expired
func (rf *Raft) isElectionTimeoutLocked() bool {
	return time.Since(rf.electionStart) > rf.electionTimeout
}

// check whether my last log is more up to date than the candidate's last log
func (rf *Raft) isMoreUpToDate(candidateIndex, candidateTerm int) bool {
	l := len(rf.log)
	// During initialization, we add an empty entry to the log, so there is no need to corner check
	lastIndex, lastTerm := l-1, rf.log[l-1].Term

	LOG(rf.me, rf.currentTerm, DVote, "Compare last log, Me: [%d]T%d, Candidate: [%d]T%d", lastIndex, lastTerm, candidateIndex, candidateTerm)
	if lastTerm != candidateTerm {
		return lastTerm > candidateTerm
	}
	return lastIndex > candidateIndex
}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (PartA, PartB).
	Term        int // candidate's term
	CandidateId int // candidate requesting vote

	LastLogIndex int // index of candidate's last log entry
	LastLogTerm  int // term of candidate's last log entry
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (PartA).
	Term        int  // 当前任期号，for candidate to update itself
	VoteGranted bool // true means candidate received vote
}

// example RequestVote RPC handler.(Recive RequestVote RPC)
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (PartA, PartB).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// reply initilization for return early
	reply.Term = rf.currentTerm
	reply.VoteGranted = false

	// 1.align the term
	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DVote, "-> S%d, Reject voted, higher term, T%d>T%d", args.CandidateId, rf.currentTerm, args.Term)
		reply.VoteGranted = false
		return
	}

	// 2.term check
	if args.Term > rf.currentTerm {
		rf.becomeFollowerLocked(args.Term)
	}

	// 3.check whether voted for other candidate
	if rf.votedFor != -1 {
		LOG(rf.me, rf.currentTerm, DVote, "-> S%d, Reject voted, Already voted to S%d", args.CandidateId, rf.votedFor)
		return
	}

	// 4. check if candidate's last log is more up to date
	if rf.isMoreUpToDate(args.LastLogIndex, args.LastLogTerm) {
		LOG(rf.me, rf.currentTerm, DVote, "-> S%d, Reject voted, Candidate's log is less up to date", args.CandidateId)
		return
	}

	reply.VoteGranted = true
	rf.votedFor = args.CandidateId
	rf.resetElectionTimerLocked()
	LOG(rf.me, rf.currentTerm, DVote, "-> S%d, Vote granted", args.CandidateId)
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

func (rf *Raft) startElection(term int) bool {
	// vote count
	votes := 0
	askVoteFromPeer := func(peer int, args *RequestVoteArgs) {
		reply := &RequestVoteReply{}
		ok := rf.sendRequestVote(peer, args, reply)

		// handle the response
		rf.mu.Lock()
		defer rf.mu.Unlock()
		if !ok {
			LOG(rf.me, rf.currentTerm, DDebug, "Ask vote from S%d, Lost or error", peer)
			return
		}
		// align term
		if reply.Term > rf.currentTerm {
			rf.becomeFollowerLocked(reply.Term)
			return
		}

		// check the context
		if rf.contextLostLocked(Candidator, term) {
			LOG(rf.me, rf.currentTerm, DVote, "Lost context, abort RequestVoteReply from S%d", peer)
			return
		}

		// count the votes
		if reply.VoteGranted {
			votes++
			if votes > len(rf.peers)/2 {
				rf.becomeLeaderLocked()
				go rf.replicationTicker(term)
			}
		}
	}

	// peer 列表（并发安全）
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.contextLostLocked(Candidator, term) {
		LOG(rf.me, rf.currentTerm, DVote, "Lost Candidator to %s, abort RequesetVote", rf.role)
		return false
	}

	l := len(rf.log)
	for peer := 0; peer < len(rf.peers); peer++ {
		if peer == rf.me {
			votes++
			continue
		}
		// 发送 RequestVote RPC
		args := &RequestVoteArgs{
			Term:         term,
			CandidateId:  rf.me,
			LastLogIndex: l - 1,
			LastLogTerm:  rf.log[l-1].Term,
		}
		go askVoteFromPeer(peer, args)
	}

	return true
}

func (rf *Raft) electionTicker() {
	for !rf.killed() {

		// Your code here (PartA)
		// Check if a leader election should be started.
		rf.mu.Lock()
		if rf.role != Leader && rf.isElectionTimeoutLocked() {
			rf.becomeCandidateLocked()
			// becomeCandidateLocker() 之后，currentTerm 加 1，role 变为 Candidator，已经为自己投票
			go rf.startElection(rf.currentTerm)
		}
		rf.mu.Unlock()

		// pause for a random amount of time between 50 and 350
		// milliseconds.
		ms := 50 + (rand.Int63() % 300)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}
