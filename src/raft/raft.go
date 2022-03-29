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
	"bytes"
	"math"
	"math/rand"
	"time"

	//	"bytes"
	"sync"
	"sync/atomic"

	//	"6.824/labgob"
	"6.824/labgob"
	"6.824/labrpc"
)

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 2D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 2D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}
type Log struct {
	Term    int // 收到日志时的currentTerm
	Command interface{}
}

var heartbeatTimeout = 100 // 心跳间隔，每100ms发送一次心跳
var FOLLOWER = "FOLLOWER"
var CANDIDATE = "CANDIDATE"
var LEADER = "LEADER"

func randomTime() int {
	return rand.Intn(heartbeatTimeout) + 2*heartbeatTimeout // 返回2～3倍的heartbeatTime
}

// 需要锁  将逻辑地址转为真实地址
func (rf *Raft) getRealIndex(index int) int {

	return index - rf.lastIncludedIndex
}

// 需要锁 将真实地址转为逻辑地址
func (rf *Raft) transfer(index int) int {

	return index + rf.lastIncludedIndex
}

// Vote 用于接收投票结果的chan
type Vote struct {
	sendTerm   int // 记录请求发送时Candidate的term
	voteGrant  bool
	term       int
	followerId int
}

// Append 用于接收Append结果
type Append struct {
	sendTerm     int // 记录请求发送时，leader的term，当接收到回应时，leader的term改变了，忽略回应
	term         int
	success      bool
	followerId   int
	lastLogIndex int
	lastLogTerm  int
	// AE包的内容。返回给发送着
	preLogIndex int
	logLen      int
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// persistent on all servers
	currentTerm int
	votedFor    int   // 投票
	log         []Log // log的index从1开始
	// 最后一个上报给Tester的log Index
	// lastApplyMsgIndex int

	// volatile on all servers
	commitIndex int // server中最后一个commited log的下标， 初始化为0， 表示没有commited的log
	lastApplied int // server中最高的已经应用到状态机的日志下标， 初始化为0

	// volatile on leader
	nextIndex  []int // leader给每一个follower维护的，保存了follower需要接受的下一个log的index, 初始化为leader的最后一个log下标+1
	matchIndex []int // 记录每一个follower中，与leader的log匹配的log的最高index， 初始化为0

	// 选举
	electTimeout int
	// 状态
	state string // F L C
	// 心跳
	heartbeatChan chan bool
	// commit
	commitChan chan int
	// vote
	voteCount int
	// 确认收到应答的数量
	// arrivedVote          int
	// arrivedAppendEntries int

	// AE回应chan
	appendReplyChan chan Append
	// vote回应chan
	requestVoteChan chan Vote
	// 上报给Tester的chan
	applyCh chan ApplyMsg
	// 终止chan
	killChan chan bool
	// 2D
	lastIncludedIndex int
	lastIncludedTerm  int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	term = rf.currentTerm
	isleader = rf.state == LEADER
	return term, isleader
}

// 需要锁 获取server需要保存的所有状态
func (rf *Raft) getRaftState() []byte {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	// e.Encode(rf.lastApplied)
	e.Encode(rf.lastIncludedIndex)
	e.Encode(rf.lastIncludedTerm)
	data := w.Bytes()
	return data
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
// 需要锁
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
	data := rf.getRaftState()
	rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
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
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var term int
	var votedFor int
	var log []Log
	// var lastApplied int
	var lastIncludedIndex int
	var lastIncludedTerm int
	if d.Decode(&term) != nil || d.Decode(&votedFor) != nil || d.Decode(&log) != nil || d.Decode(&lastIncludedIndex) != nil || d.Decode(&lastIncludedTerm) != nil {

	} else {
		rf.mu.Lock()
		rf.currentTerm = term
		rf.votedFor = votedFor
		rf.log = log
		// rf.lastApplied = lastApplied
		rf.lastIncludedIndex = lastIncludedIndex
		rf.lastIncludedTerm = lastIncludedTerm
		rf.lastApplied = lastIncludedIndex
		// rf.log[0].Term = rf.lastIncludedTerm
		rf.mu.Unlock()
	}
}

//
// A service wants to switch to snapshot.  Only do so if Raft hasn't
// have more recent info since it communicate the snapshot on applyCh.
//
func (rf *Raft) CondInstallSnapshot(lastIncludedTerm int, lastIncludedIndex int, snapshot []byte) bool {
	EntrantOutPrintf("Server%d entrant CondInstallSnapshot", rf.me)
	defer EntrantOutPrintf("Server%d out CondInstallSnapshot", rf.me)
	// Your code here (2D).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	Lab2DPrintf("CondInstallSnapshot: Service request switch to snapshot at Server %d, LastIncludedIndex=%d, LastIncludedTerm=%d", rf.me, lastIncludedIndex, lastIncludedTerm)
	if rf.commitIndex >= lastIncludedIndex {
		Lab2DPrintf("Server %d disagree to snapshot, lastIncludedIndex=%d, commitIndex=%d, log_len=%d", rf.me, rf.lastIncludedIndex, rf.commitIndex, len(rf.log))
		return false
	}
	if lastIncludedIndex >= rf.transfer(len(rf.log)-1) {
		rf.log = make([]Log, 1)
	} else {
		start := rf.getRealIndex(lastIncludedIndex)
		rf.log = append([]Log{}, rf.log[start:]...)
		rf.log[0].Command = nil
	}
	rf.commitIndex, rf.lastIncludedIndex, rf.lastApplied = lastIncludedIndex, lastIncludedIndex, lastIncludedIndex
	rf.log[0].Term = lastIncludedTerm
	rf.persister.SaveStateAndSnapshot(rf.getRaftState(), snapshot)
	Lab2DPrintf("Server %d agree to snapshot, lastIncludedIndex=%d, commitIndex=%d, log_len=%d", rf.me, rf.lastIncludedIndex, rf.commitIndex, len(rf.log))

	return true
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).
	EntrantOutPrintf("Server%d entrant Snapshot", rf.me)
	defer EntrantOutPrintf("Server%d out Snapshot", rf.me)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// 已经有更高的快照版本
	if index <= rf.lastIncludedIndex {
		return
	}
	i := rf.getRealIndex(index)
	// 切断原log的引用
	rf.log = append([]Log{}, rf.log[i:]...)
	rf.log[0].Command = nil
	rf.lastIncludedIndex = index
	rf.lastIncludedTerm = rf.log[0].Term
	if rf.lastApplied < rf.lastIncludedIndex {
		rf.lastApplied = rf.lastIncludedIndex
	}
	data := rf.getRaftState()
	rf.persister.SaveStateAndSnapshot(data, snapshot)
	Lab2DPrintf("Server %d accept snapshot from service, lastIncludedIndex=%d, commitIndex=%d, log_len=%d", rf.me, index, rf.commitIndex, len(rf.log))
}

type InstallSnapshotArgs struct {
	Term              int
	LeaderId          int
	LastIncludedIndex int
	LastIncludedTerm  int
	Data              []byte
}
type InstallSnapshotReply struct {
	Term int
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateId  int
	LastLogIndex int // candidate的最后一个log的index
	LastLogTerm  int // candidate的最后一个log的term； 与上面的变量一起，用来比较log。follower根据比较结果决定要不用投票
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}
type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int // nextIndex - 1
	PrevLogTerm  int // nextIndex - 1对应的term
	Entry        []Log
	LeaderCommit int
}
type AppendEntriesReply struct {
	Term         int
	Success      bool // true if f contained entry matching prevLodIndex ans prevLogTerm
	LastLogIndex int  // 给Leader上报最后一条日志的信息
	LastLogTerm  int
}

// 处理InstallSnapshot
func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	EntrantOutPrintf("Server%d entrant InstallSnapshot", rf.me)
	defer EntrantOutPrintf("Server%d out InstallSnapshot", rf.me)
	if rf.killed() {
		return
	}
	rf.mu.Lock()
	reply.Term = rf.currentTerm
	if rf.currentTerm > args.Term {
		rf.mu.Unlock()
		return
	}
	Lab2DPrintf("Server %d receive snapshot from Server %d, LastIncludedIndex=%d, LastIncludedTerm=%d", rf.me, args.LeaderId, args.LastIncludedIndex, args.LastIncludedTerm)
	if rf.currentTerm < args.Term {
		rf.toFollower(args.Term, -1)
	}
	select {
	case rf.heartbeatChan <- true:
	case <-rf.killChan:
		return
	default:
	}
	// 收到旧的shot
	if rf.commitIndex >= args.LastIncludedIndex {
		rf.mu.Unlock()
		return
	}
	rf.mu.Unlock()
	go func() {
		applyMsg := ApplyMsg{
			SnapshotValid: true,
			Snapshot:      args.Data,
			SnapshotTerm:  args.LastIncludedTerm,
			SnapshotIndex: args.LastIncludedIndex,
		}
		rf.applyCh <- applyMsg
	}()
}

// RequestVote
// example RequestVote RPC handler.
//
// 处理投票请求
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	EntrantOutPrintf("Server%d entrant RequestVote", rf.me)
	defer EntrantOutPrintf("Server%d out RequestVote", rf.me)
	if rf.killed() {
		return
	}
	rf.mu.Lock()
	defer rf.mu.Unlock()
	reply.VoteGranted = false
	reply.Term = rf.currentTerm
	if rf.currentTerm < args.Term {
		rf.toFollower(args.Term, args.CandidateId)
	}
	// DPrintf("[Server:%d, term:%d, state=%s] 收到来自 [Server:%d, term:%d] 的投票请求", rf.me, rf.currentTerm, rf.state, args.CandidateId, args.Term)
	// 当前term更大。 或者已经给通term的其他candidate投过票
	if rf.currentTerm > args.Term || rf.votedFor > -1 && rf.votedFor != args.CandidateId && rf.currentTerm == args.Term {
		DPrintf("[Server:%d, term:%d, state:%s, log_len:%d] 拒绝来自 [Server:%d, term:%d, log_len:%d] 的投票请求，因为其term或已经投过票", rf.me, rf.currentTerm, rf.state, len(rf.log), args.CandidateId, args.Term, args.LastLogIndex+1)
		if rf.currentTerm < args.Term {
			rf.votedFor = -1
			rf.persist()
		}
		return
	}
	// 当前的日志更新
	if rf.freshThan(args.LastLogTerm, args.LastLogIndex) {
		DPrintf("[Server:%d, term:%d, state:%s, log_len:%d] 拒绝来自 [Server:%d, term:%d, log_len:%d] 的投票请求： 因为其旧日志", rf.me, rf.currentTerm, rf.state, len(rf.log), args.CandidateId, args.Term, args.LastLogIndex+1)
		if rf.currentTerm < args.Term {
			rf.votedFor = -1
			rf.persist()
		}
		return
	}
	DPrintf("[Server:%d, term:%d, state:%s, log_len:%d] 同意来自 [Server:%d, term:%d, log_len:%d] 的投票请求", rf.me, rf.currentTerm, rf.state, len(rf.log), args.CandidateId, args.Term, args.LastLogIndex+1)
	reply.VoteGranted = true
	// DPrintf("[Server:%d, term:%d, state:%s] 同意来自 [Server:%d, term:%d] 的投票请求", rf.me, rf.currentTerm, rf.state, args.CandidateId, args.Term)
	// 重置超时选举计时,因为已经给别人投票，所以自己不会参与竞选
	select {
	case rf.heartbeatChan <- true:
	case <-rf.killChan:
		rf.killChan <- true
		return
	default:
	}
}

// AppendEntries Follower处理心跳和追加log
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	EntrantOutPrintf("Server%d entrant AppendEntries", rf.me)
	defer EntrantOutPrintf("Server%d out AppendEntries", rf.me)
	if rf.killed() {
		return
	}
	rf.mu.Lock()
	defer rf.mu.Unlock()
	reply.Term = rf.currentTerm
	if rf.currentTerm > args.Term {
		return
	}
	if rf.currentTerm == args.Term && rf.state == LEADER {
		return
	}
	// Success = false
	if rf.currentTerm <= args.Term {
		if rf.state == CANDIDATE && rf.currentTerm == args.Term {
			rf.toFollower(args.Term, -1)
		} else if rf.currentTerm < args.Term {
			rf.toFollower(args.Term, -1)
		}
	}
	// 对于过时的AE包，不重置超时选举计时
	select {
	case rf.heartbeatChan <- true:
	default:
	}
	if len(args.Entry) > 0 {
		BPrintf("[Server:%d, term:%d, state:%s] 收到来自Leader[Server:%d, term:%d]的AppendEntries, len=%d", rf.me, rf.currentTerm, rf.state, args.LeaderId, args.Term, len(args.Entry))
	}
	// 将rf.getRealIndex转换为实际prevLogIndex, [0,len-1]
	prevLogIndex := rf.getRealIndex(args.PrevLogIndex)
	// prevLogIndex已经trim掉, 或者太大
	//// leader应该把nextIndex设置为reply.LastLogIndex
	if prevLogIndex < 0 || len(rf.log) <= prevLogIndex {
		// reply.LastLogIndex = len(rf.log) - 1
		// reply.LastLogTerm = rf.log[reply.LastLogIndex].Term
		reply.LastLogIndex = rf.transfer(len(rf.log))
		reply.LastLogTerm = -1
		return
	}
	// nextIndex有两种可能：1、当前Term的最后一个index+1  2、lastLogIndex
	if rf.log[prevLogIndex].Term != args.PrevLogTerm {
		conflictTerm := rf.log[prevLogIndex].Term
		// 找到第一个term = conflictTerm 的log的下标
		i := 0
		for ; i < prevLogIndex; i++ {
			if rf.log[i].Term == conflictTerm {
				break
			}
		}
		reply.LastLogIndex = rf.transfer(i)
		reply.LastLogTerm = conflictTerm
		return
	}
	// Success = true
	// 处理日志复制
	// 假设不需要保留Entry.log
	reply.Success = true
	reply.LastLogIndex = rf.transfer(len(rf.log) - 1)
	reply.LastLogTerm = rf.log[len(rf.log)-1].Term
	// 是否见过， 或者是否是心跳
	hold := false
	if len(args.Entry) == 0 {
		goto updateCommitIndex
	}
	// 检查是否需要保留Entry,也就是检查AE是不是有新内容
	// leader:[0,1,1,2,2] , follower:[0,1,1,2], entry=[2], pli = 3, ni = 4
	// leader:[0,1,1,2,2] , follower:[0,1,1,2,2], entry=[2,2], pli = 2, ni = 3 : 已经见过
	for i := prevLogIndex + 1; i < len(rf.log) && (i-prevLogIndex-1) < len(args.Entry); i++ {
		// if rf.log[i].Term != args.Entry[i-args.PrevLogIndex-1].Term || rf.log[i].Command != args.Entry[i-args.PrevLogIndex-1].Command {
		// 	hold = true
		// 	break
		// }
		if rf.log[i].Term != args.Entry[i-prevLogIndex-1].Term {
			hold = true
			break
		}
	}
	// 如果Entry有新内容，或者Entry是直接追加到log后面；或者AE包前面跟follower一样，后面还有更多
	if hold || len(rf.log) == prevLogIndex+1 {
		rf.log = rf.log[0 : prevLogIndex+1] // 保留[0, args.PrevLogIndex]的log
		rf.log = append(rf.log, args.Entry...)
		reply.LastLogIndex = rf.transfer(len(rf.log) - 1)
		reply.LastLogTerm = rf.log[len(rf.log)-1].Term
		rf.persist()
	} else if len(rf.log)-1-prevLogIndex < len(args.Entry) { //AE包前面跟follower一样，后面还有更多
		// start := len(rf.log) - args.PrevLogIndex - 1
		// rf.log = append(rf.log, args.Entry[start:]...)
		rf.log = rf.log[:prevLogIndex+1]
		rf.log = append(rf.log, args.Entry...)
		reply.LastLogIndex = rf.transfer(len(rf.log) - 1)
		reply.LastLogTerm = rf.log[len(rf.log)-1].Term
		rf.persist()
	}
updateCommitIndex:
	if args.LeaderCommit > rf.commitIndex {
		oldCommit := rf.commitIndex
		rf.commitIndex = int(math.Min(float64(args.LeaderCommit), float64(args.PrevLogIndex+len(args.Entry))))
		if oldCommit > rf.commitIndex || rf.currentTerm != rf.log[rf.getRealIndex(rf.commitIndex)].Term {
			rf.commitIndex = oldCommit
		}
		go rf.confirmNewCommit(oldCommit, rf.commitIndex)
	}
	rf.printLogState()
}

//
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
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	EntrantOutPrintf("Server%d entrant sendRequestVote", rf.me)
	defer EntrantOutPrintf("Server%d out sendRequestVote", rf.me)
	if rf.killed() {
		return false
	}
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	if !ok {
		return ok
	}
	vote := Vote{voteGrant: reply.VoteGranted, term: reply.Term, followerId: server, sendTerm: args.Term}
	select {
	case rf.requestVoteChan <- vote:
	case <-rf.killChan:
		rf.killChan <- true
		return false
	}
	return ok
}
func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	EntrantOutPrintf("Server%d entrant sendAppendEntries", rf.me)
	defer EntrantOutPrintf("Server%d out sendAppendEntries", rf.me)
	if rf.killed() {
		return false
	}
	append := Append{
		sendTerm: args.Term,
	}
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	if !ok {
		return ok
	}
	append.success = reply.Success
	append.term = reply.Term
	append.followerId = server
	append.lastLogIndex = reply.LastLogIndex
	append.lastLogTerm = reply.LastLogTerm
	append.preLogIndex = args.PrevLogIndex
	append.logLen = len(args.Entry)
	// rf.appendReplyChan <- append
	select {
	case rf.appendReplyChan <- append:
	case <-rf.killChan:
		rf.killChan <- true
		return false
	}
	return ok
}
func (rf *Raft) sendInstallSnapshot(server int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) bool {
	EntrantOutPrintf("Server%d entrant sendInstallSnapshot", rf.me)
	defer EntrantOutPrintf("Server%d out sendInstallSnapshot", rf.me)
	if rf.killed() {
		return false
	}
	ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
	return ok
}

//
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
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	EntrantOutPrintf("Server%d entrant Start", rf.me)
	defer EntrantOutPrintf("Server%d out Start", rf.me)
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).
	rf.mu.Lock()
	if rf.state != LEADER {
		rf.mu.Unlock()
		return index, term, false
	}
	newLog := Log{
		Term:    rf.currentTerm,
		Command: command,
	}
	rf.log = append(rf.log, newLog)
	rf.persist()

	// index = len(rf.log)
	index = rf.transfer(len(rf.log) - 1)
	term = rf.currentTerm
	TesterPrintf("[Server:%d, term:%d, state:%s, log_len:%d, lastIncludedIndex:%d]取得新日志:[Command: %v, index: %d]", rf.me, rf.currentTerm, rf.state, len(rf.log), rf.lastIncludedIndex, command, index)
	rf.mu.Unlock()
	// 发送AppendEntries给所有follower
	go rf.broadcastNewLog()
	return index, term, isLeader
}

//
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
	// 清空所有的chan
	go rf.end()

}
func (rf *Raft) end() {
	rf.killChan <- true
	// close(rf.appendReplyChan)
	// close(rf.applyCh)
	// close(rf.commitChan)
	// close(rf.heartbeatChan)
}
func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// The ticker go routine starts a new election if this peer hasn't received
// heartsbeats recently.
func (rf *Raft) ticker() {
	for !rf.killed() {
		// Your code here to check if a leader election should
		// be started and to randomize sleeping time using
		// time.Sleep().
		for !rf.killed() {
			rf.mu.Lock()
			electTimeout := rf.electTimeout
			rf.mu.Unlock()
			select {
			case <-rf.heartbeatChan: // 收到心跳就重置
			case <-time.After(time.Duration(electTimeout) * time.Millisecond):
				// 开启选举
				go rf.startElect()
			case <-rf.killChan:
				rf.killChan <- true
				return
			}
		}
	}
}
func (rf *Raft) startElect() {
	EntrantOutPrintf("Server%d entrant startElect", rf.me)
	defer EntrantOutPrintf("Server%d out startElect", rf.me)
	if rf.killed() {
		return
	}
	rf.mu.Lock()
	if rf.state == LEADER {
		rf.mu.Unlock()
		return
	}
	DPrintf("[Server:%d, term:%d, state:%s, log_len:%d] 经过选举超时未收到心跳，发起选举", rf.me, rf.currentTerm, rf.state, len(rf.log))
	rf.toCandidate()
	rf.mu.Unlock()
	for server := 0; server < len(rf.peers); server++ {
		if server == rf.me {
			continue
		}
		go rf.requestVote(server)
	}
}

func (rf *Raft) requestVote(server int) {
	EntrantOutPrintf("Server%d entrant requestVote", rf.me)
	defer EntrantOutPrintf("Server%d out requestVote", rf.me)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	args := RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: rf.transfer(len(rf.log)) - 1,
		LastLogTerm:  rf.log[len(rf.log)-1].Term,
	}
	reply := RequestVoteReply{
		Term:        0,
		VoteGranted: false,
	}
	go rf.sendRequestVote(server, &args, &reply)
}
func (rf *Raft) manageRequestVote() {
	for !rf.killed() {
		EntrantOutPrintf("Server%d entrant manageRequestVote", rf.me)
		var reply Vote
		select {
		case reply = <-rf.requestVoteChan:
		case <-rf.killChan:
			rf.killChan <- true
			return
		}
		// reply := <-rf.requestVoteChan
		rf.mu.Lock()
		if reply.term > rf.currentTerm {
			rf.toFollower(reply.term, -1)
			rf.mu.Unlock()
			continue
		}
		if rf.currentTerm != reply.sendTerm || rf.state != CANDIDATE {
			rf.mu.Unlock()
			continue
		}
		if reply.voteGrant {
			rf.voteCount++
			if rf.voteCount == len(rf.peers)/2+1 && rf.state == CANDIDATE {
				rf.toLeader()
			}
		}
		rf.mu.Unlock()
		EntrantOutPrintf("Server%d out manageRequestVote", rf.me)
	}
}

// 需要锁，比较日志的新旧
func (rf *Raft) freshThan(term int, index int) bool {
	last := len(rf.log)
	// rf的log为空
	if last == 1 {
		return false
	}
	// candidate的log为空
	if term == 0 {
		return true
	}
	// 两者的log都不空
	// 比较最后一条日志的新旧程度
	if rf.log[last-1].Term < term || rf.log[last-1].Term == term && index >= rf.transfer(last-1) {
		return false
	}
	return true
}

// 需要锁
func (rf *Raft) toCandidate() {
	rf.state = CANDIDATE
	rf.votedFor = rf.me
	rf.voteCount = 1
	rf.currentTerm++
	rf.electTimeout = randomTime()
	rf.persist()
	go rf.manageRequestVote()
	DPrintf("[Server:%d, term:%d, state:%s, log_len:%d] 转变为CANDIDATE", rf.me, rf.currentTerm, rf.state, len(rf.log))
}

// 需要锁
func (rf *Raft) toFollower(term int, vote int) {
	DPrintf("[Server:%d, term:%d, state:%s, log_len:%d] 转变为FOLLOWER", rf.me, rf.currentTerm, rf.state, len(rf.log))
	rf.state = FOLLOWER
	rf.currentTerm = term
	rf.votedFor = vote
	rf.voteCount = 0
	rf.persist()
}

// 需要锁
func (rf *Raft) toLeader() {
	rf.state = LEADER
	rf.voteCount = 0
	// 新Leader默认nextIndex为最后一个log的Index+1
	// matchIndex默认为0
	for i := 0; i < len(rf.peers); i++ {
		rf.nextIndex[i] = rf.transfer(len(rf.log))
		rf.matchIndex[i] = 0
	}
	// 开启心跳
	go rf.heartbeat()
	// 进行AE应答管理
	go rf.manageAppendEntriesReply()
	// 尝试更新CommitIndex
	go rf.tryUpdateLeaderCommitIndex()
	// rf.persist()
	DPrintf("[Server:%d, term:%d, state:%s, log_len:%d] 被选举为LEADER", rf.me, rf.currentTerm, rf.state, len(rf.log))
}

// 定期发送心跳
func (rf *Raft) heartbeat() {
	for !rf.killed() {
		rf.mu.Lock()
		state := rf.state
		// term := rf.currentTerm
		rf.mu.Unlock()
		if state != LEADER {
			//DPrintf("[Server:%d, term:%d, state=%s] 不再是Leader，不再发送心跳", rf.me, rf.currentTerm, rf.state)
			return
		}
		// DPrintf("[Server:%d, term:%d, state=%s] 广播心跳", rf.me, term, state)
		go rf.sendHeartbeat()
		time.Sleep(time.Duration(heartbeatTimeout) * time.Millisecond)

		// DPrintf("[Server:%d, term:%d, state=%s] 醒来，准备重新发送心跳", rf.me, rf.currentTerm, rf.state)
	}
}

func (rf *Raft) sendHeartbeat() {
	EntrantOutPrintf("Server%d entrant sendHeartbeat", rf.me)
	defer EntrantOutPrintf("Server%d out sendHeartbeat", rf.me)
	rf.mu.Lock()
	if rf.state != LEADER {
		rf.mu.Unlock()
		return
	}
	rf.mu.Unlock()
	//heartbeatChan := make(chan Append, len(rf.peers))
	for server := 0; server < len(rf.peers); server++ {
		if server == rf.me {
			continue
		}
		go rf.appendEntries(server)
	}
}

// 向follower发送AE
func (rf *Raft) appendEntries(server int) {
	EntrantOutPrintf("Server%d entrant appendEntries", rf.me)
	defer EntrantOutPrintf("Server%d out appendEntries", rf.me)
	if rf.killed() {
		return
	}
	rf.mu.Lock()
	// 此时发送snapshot，并处理回应
	if rf.nextIndex[server] <= rf.lastIncludedIndex {
		args := &InstallSnapshotArgs{
			Term:              rf.currentTerm,
			LeaderId:          rf.me,
			LastIncludedIndex: rf.lastIncludedIndex,
			LastIncludedTerm:  rf.lastIncludedTerm,
			Data:              rf.persister.snapshot,
		}
		reply := &InstallSnapshotReply{
			Term: 0,
		}
		Lab2DPrintf("Server %d send snapshot to Server %d, LastIncludedIndex=%d, LastIncludedTerm=%d", rf.me, server, rf.lastIncludedIndex, rf.lastIncludedTerm)
		rf.mu.Unlock()
		go func() {
			ok := rf.sendInstallSnapshot(server, args, reply)
			if ok {
				rf.mu.Lock()
				if reply.Term > rf.currentTerm {
					rf.toFollower(reply.Term, -1)
				}
				rf.nextIndex[server] = rf.transfer(len(rf.log))
				if rf.matchIndex[server] < args.LastIncludedIndex {
					rf.matchIndex[server] = args.LastIncludedIndex
				}
				rf.mu.Unlock()
			}
		}()
		return
	}
	var entry []Log
	var pli int
	var plt int
	// entry 为空: 发生在Leader刚被选举，并且没有收到client请求 ,此时rf.nextIndex[server] = len(rf.log)
	if rf.nextIndex[server] >= rf.transfer(len(rf.log)) {
		pli = rf.transfer(len(rf.log)) - 1
		plt = rf.log[rf.getRealIndex(pli)].Term
		entry = make([]Log, 0)
	} else {
		// 深拷贝
		temp := make([]Log, 0)
		temp = append(temp, rf.log[rf.getRealIndex(rf.nextIndex[server]):]...)
		entry = make([]Log, len(temp))
		copy(entry, temp)
		// entry = append(entry, rf.log[rf.nextIndex[server]:]...)
		pli = rf.nextIndex[server] - 1
		if pli < 0 {
			pli = 0
		}
		plt = rf.log[rf.getRealIndex(pli)].Term
	}
	args := AppendEntriesArgs{
		Term:         rf.currentTerm,
		LeaderId:     rf.me,
		PrevLogIndex: pli,
		PrevLogTerm:  plt,
		Entry:        entry,
		LeaderCommit: rf.commitIndex,
	}
	BPrintf("[Server:%d, term:%d, state=%s] 发送AE包给 [Server:%d], 包内容:[PrevLogIndex:%d, PrevLogTerm:%d, Log len:%d]", rf.me, rf.currentTerm, rf.state, server, args.PrevLogIndex, args.PrevLogTerm, len(args.Entry))
	rf.mu.Unlock()
	reply := AppendEntriesReply{
		Term:    0,
		Success: false,
	}
	// DPrintf("[Server:%d, term:%d, state=%s] 发送AE包给 [Server:%d]", rf.me, rf.currentTerm, rf.state, server)
	go rf.sendAppendEntries(server, &args, &reply)
}

// 给所有follower广播新的log，同时要进行commit
func (rf *Raft) broadcastNewLog() {
	EntrantOutPrintf("Server%d entrant broadcastNewLog", rf.me)
	defer EntrantOutPrintf("Server%d out broadcastNewLog", rf.me)
	// 发送AppendEntries
	//appendChan := make(chan Append, len(rf.peers))
	if rf.killed() {
		return
	}
	for server := 0; server < len(rf.peers); server++ {
		if server == rf.me {
			continue
		}
		rf.appendEntries(server)
	}
	//rf.waitReply(oldLen, oldTerm, appendChan, false)
}

// 负责接收AE的回应
func (rf *Raft) manageAppendEntriesReply() {
	for !rf.killed() {
		EntrantOutPrintf("Server%d entrant manageAppendEntriesReply", rf.me)
		// 如果RPC调用无回复，就不会向chan里放回复
		// reply := <-rf.appendReplyChan
		var reply Append
		select {
		case reply = <-rf.appendReplyChan:
		case <-rf.killChan:
			rf.killChan <- true
			return
		}
		rf.mu.Lock()
		BPrintf("[Server:%d, term:%d, state:%s] 收到来自[Server:%d]的AE回应", rf.me, rf.currentTerm, rf.state, reply.followerId)
		if rf.currentTerm < reply.term {
			rf.toFollower(reply.term, -1)
			rf.mu.Unlock()
			continue
		}
		// 处理过期的AE回复: 网络延时过高导致,比如一条更新log失败的回复，一直没有到达，导致log被新的AE包更新。此时，应该丢弃这条回复。
		if rf.currentTerm != reply.sendTerm {
			rf.mu.Unlock()
			continue
		}
		if rf.state != LEADER {
			rf.mu.Unlock()
			continue
		}
		if !reply.success {
			// rf.nextIndex[reply.followerId] = reply.lastLogIndex + 1
			if reply.lastLogTerm == -1 {
				rf.nextIndex[reply.followerId] = reply.lastLogIndex
			} else {
				// 从后向前，找第一个等于reply.lastLogTerm的下一个index
				var i int
				for i = len(rf.log) - 1; i > 0; i-- {
					if rf.log[i].Term == reply.lastLogTerm {
						break
					}
				}
				if i == 0 {
					rf.nextIndex[reply.followerId] = reply.lastLogIndex
				} else {
					rf.nextIndex[reply.followerId] = rf.transfer(i + 1)
				}
			}
			go rf.appendEntries(reply.followerId)
		} else {
			if reply.preLogIndex+reply.logLen > rf.matchIndex[reply.followerId] {
				rf.matchIndex[reply.followerId] = reply.preLogIndex + reply.logLen
			}
			rf.nextIndex[reply.followerId] = rf.transfer(len(rf.log))

			// 只能commit当前Term的log
			if rf.matchIndex[reply.followerId] > rf.commitIndex && rf.currentTerm == rf.log[rf.getRealIndex(rf.matchIndex[reply.followerId])].Term {
				rf.trySendCommitChan(rf.matchIndex[reply.followerId])
				rf.printLogState()
			}
			if rf.matchIndex[reply.followerId] < rf.transfer(len(rf.log)-1) {
				// 没有跟上，继续跟进; 有一种可能导致这种情况
				// 1、发送AE后，Leader的log又更新了
				go rf.appendEntries(reply.followerId)
			}
		}
		rf.printLogState()
		rf.mu.Unlock()
		EntrantOutPrintf("Server%d out manageAppendEntriesReply", rf.me)

	}
}

// 尝试更新Leader的commitIndex
// 每当有一个follower的日志跟上Leader的日志时调用
func (rf *Raft) tryUpdateLeaderCommitIndex() {
	for !rf.killed() {
		EntrantOutPrintf("Server%d entrant tryUpdateLeaderCommitIndex", rf.me)
		// matchIndex := <-rf.commitChan
		var matchIndex int
		select {
		case matchIndex = <-rf.commitChan:
		case <-rf.killChan:
			rf.killChan <- true
			return
		}
		// 检查当前是否可以commit
		rf.mu.Lock()
		if rf.state != LEADER {
			rf.mu.Unlock()
			continue
		}
		oldCommit := rf.commitIndex
		BPrintf("[Server:%d, term:%d, state:%s] 尝试更新CommitIndex", rf.me, rf.currentTerm, rf.state)
		// 当前无需更新commit
		if rf.commitIndex == rf.transfer(len(rf.log)-1) || rf.commitIndex >= matchIndex || matchIndex >= rf.transfer(len(rf.log)) {
			rf.mu.Unlock()
			continue
		}
		// 检查当前最新的matchIndex是否可以commit
		cnt := 1
		canCommit := false
		for i := 0; i < len(rf.peers); i++ {
			if i == rf.me {
				continue
			}
			if rf.matchIndex[i] >= matchIndex {
				cnt++
				if cnt >= len(rf.peers)/2+1 {
					canCommit = true
					break
				}
			}
		}
		if canCommit && rf.currentTerm == rf.log[rf.getRealIndex(matchIndex)].Term {
			rf.commitIndex = matchIndex
		}
		newCommit := rf.commitIndex
		rf.printLogState()
		rf.mu.Unlock()
		go rf.confirmNewCommit(oldCommit, newCommit)
		EntrantOutPrintf("Server%d out tryUpdateLeaderCommitIndex", rf.me)

	}
}

// 周期性检查commit 和 applied
// func (rf *Raft) tryApply() {
// 	for !rf.killed() {
// 		rf.mu.Lock()
// 		if rf.lastApplied < rf.commitIndex {
// 			rf.lastApplied++
// 			// rf.persister.SaveRaftState(rf.getRaftState())
// 		}
// 		command := rf.log[rf.getRealIndex(rf.lastApplied)].Command
// 		applyMsg := ApplyMsg{
// 			CommandValid: true,
// 			CommandIndex: rf.lastApplied,
// 			Command:      command,
// 		}
// 		rf.mu.Unlock()
// 		select {
// 		case rf.applyCh <- applyMsg:
// 		case <-rf.killChan:
// 			rf.killChan <- true
// 			return
// 		}
// 		time.Sleep(time.Millisecond)
// 	}
// }

// 通知tester有新的conmmit log
// 1.follower 在收到leader的AE包时， 更新cmomitIndex后
// 2.leader在收到follower的Ae包回应，并发现大部分follower都跟上了脚步时
func (rf *Raft) confirmNewCommit(oldCommit int, newCommit int) {
	EntrantOutPrintf("Server%d entrant confirmNewCommit", rf.me)
	defer EntrantOutPrintf("Server%d out confirmNewCommit", rf.me)
	if rf.killed() {
		return
	}
	rf.mu.Lock()
	if oldCommit >= newCommit || newCommit <= rf.lastApplied {
		rf.mu.Unlock()
		return
	}
	if newCommit == 1 || newCommit == 51 || newCommit == 101 || newCommit == 102 {
		Backup2BTestPrintf("Server=%d, state=%s, commitIndex=%d, commitTerm=%d, commitCommand=%v", rf.me, rf.state, newCommit, rf.log[rf.getRealIndex(newCommit)].Term, rf.log[rf.getRealIndex(newCommit)].Command)
	}
	Lab2DPrintf("Server%d lastIncludedIndex=%d commitIndex=%d lastApplied=%d has new Log to apply: [%d, %d]", rf.me, rf.lastIncludedIndex, rf.commitIndex, rf.lastApplied, rf.lastApplied+1, newCommit)
	entires := make([]Log, newCommit-rf.lastApplied)
	copy(entires, rf.log[rf.getRealIndex(rf.lastApplied+1):rf.getRealIndex(newCommit+1)])
	start := rf.lastApplied + 1
	rf.mu.Unlock()
	// 从rf.lastApplyMsgIndex + 1开始
	// commands := make([]interface{}, 0)
	for index, entry := range entires {
		select {
		case rf.applyCh <- ApplyMsg{
			CommandValid: true,
			Command:      entry.Command,
			CommandIndex: start + index,
		}:
		case <-rf.killChan:
			rf.killChan <- true
			return
		}
	}
	rf.mu.Lock()
	if rf.lastApplied < newCommit {
		rf.lastApplied = newCommit
	}
	TesterPrintf("[Server:%d, term:%d, state:%s, commitIndex:%d,log_len:%d, lastIncludedIndex:%d] 向Tester提供了Command[commandIndex:%d, command:%v]～Command[commandIndex:%d, command:%v]", rf.me, rf.currentTerm, rf.state, rf.commitIndex, len(rf.log), rf.lastIncludedIndex, start, entires[0].Command, start+len(entires)-1, entires[len(entires)-1].Command)
	rf.mu.Unlock()
	// for i := rf.lastApplied + 1; i <= newCommit; i++ {
	// 	rf.mu.Lock()
	// 	command := rf.log[rf.getRealIndex(i)].Command
	// 	applyMsg := ApplyMsg{
	// 		CommandValid: true,
	// 		CommandIndex: i,
	// 		Command:      command,
	// 	}

	// 	// TesterPrintf("[Server:%d, term:%d, state:%s, log_len:%d, lastIncludedIndex:%d] 准备向Tester提供Command[commandIndex:%d, command:%v]", rf.me, rf.currentTerm, rf.state, len(rf.log), rf.lastIncludedIndex, i, command)
	// 	rf.mu.Unlock()

	// 	select {
	// 	case rf.applyCh <- applyMsg:
	// 	case <-rf.killChan:
	// 		rf.killChan <- true
	// 		return
	// 	}
	// 	// rf.applyCh <- applyMsg
	// 	// rf.lastApplyMsgIndex = i
	// 	rf.mu.Lock()
	// 	TesterPrintf("[Server:%d, term:%d, state:%s, commitIndex:%d,log_len:%d, lastIncludedIndex:%d] 向Tester提供了Command[commandIndex:%d, command:%v]", rf.me, rf.currentTerm, rf.state, rf.commitIndex, len(rf.log), rf.lastIncludedIndex, i, command)
	// 	rf.lastApplied = i
	// 	rf.persist()
	// 	rf.mu.Unlock()
	// }
}

// 尝试通知tryUpdateCommitIndex可以进行commit检查
func (rf *Raft) trySendCommitChan(matchIndex int) {
	select {
	case rf.commitChan <- matchIndex:
	default:
	}
}

// 需要锁，打印server的log状态
func (rf *Raft) printLogState() {
	BPrintf("[Server:%d, term:%d, state:%s, lastLogIndex:%d, lastLogTerm:%d, commitIndex:%d, lastCommand:%v]", rf.me, rf.currentTerm, rf.state, rf.transfer(len(rf.log)-1), rf.log[len(rf.log)-1].Term, rf.commitIndex, rf.log[len(rf.log)-1].Command)
}

// 周期性的尝试进行apply
// func (rf *Raft) tryApply() {
// 	for !rf.killed() {
// 		rf.mu.Lock()
// 		// DPrintf("[Server:%d, term:%d, state:%s] 尝试更新AppliedIndex", rf.me, rf.currentTerm, rf.state)
// 		if rf.commitIndex > rf.lastApplied {
// 			rf.lastApplied = rf.commitIndex
// 		}
// 		rf.mu.Unlock()
// 		time.Sleep(10 * time.Millisecond) // 休息1ms，再检查是否需要更新
// 	}
// }

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.applyCh = applyCh // 更新commitIndex时，通知tester
	// Your initialization code here (2A, 2B, 2C).
	rf.currentTerm = 0
	rf.votedFor = -1
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.log = make([]Log, 1) // size 为1的切片,0号位不使用
	rf.log[0] = Log{
		Term: 0,
	}
	lens := len(rf.peers)
	rf.nextIndex = make([]int, lens)
	rf.matchIndex = make([]int, lens)
	rf.electTimeout = randomTime()
	rf.state = FOLLOWER
	rf.heartbeatChan = make(chan bool, 1) // AppendEntries不会被阻塞，起码能容纳一个心跳和一个正常AE
	rf.commitChan = make(chan int, 1)
	rf.appendReplyChan = make(chan Append, 2*lens)
	rf.requestVoteChan = make(chan Vote, lens)
	rf.killChan = make(chan bool, 1)
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.ticker()
	return rf
}
