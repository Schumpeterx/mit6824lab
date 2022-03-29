# mit6824lab
## LAB 2
### 概括
LAB对我来说难度还是挺高的。前前后后做了20来天，最后虽然大致都通过，但是在Lab 2B的TestCount测试和Lab 2C的Figure8 unreliable测试还是偶尔会出错。TestCount不通过的原因在于结构设计问题导致RPC调用过多。Figure8 unreliable在VScode中测试不会出错，但是在终端调用go test -run 2C -race时，偶尔会提示applied error，原因尚不明确。最后，长时间执行，还会出现内存溢出问题。应该是由于chan没有正常关闭，导致等待的协程过多。
此外还有个奇怪的问题，如果单独进行每项测试，出错概率很小（还没遇到），但是go test -run 2* -race就很容易出错。原因未知。
写代码前，要多读[Student Guide](https://thesquareplanet.com/blog/students-guide-to-raft/)，遵守里面的每一条规则。注意在提前退出函数前，一定要释放锁，不然会造成死锁。这个问题困扰我好久。另外，可以通过打日志来Debug。比如，Leader在某条日志后，不再出现，触发重新选举，那么说明Leader发生了死锁。需要仔细检查代码。
### 总体设计
使用Chan + Select + Timer来执行定时任务
使用Chan来传递RPC调用结果，Leader中固定两个协程来负责接收RPC调用的结果，分别负责处理RequestVote和AppendEntries、InstallSnapshot RPC调用的结果。在收到AE的回复时，查看follower的matchIndex是不是等于Leader的最后一个日志下标，如果不是，则继续发送AE到follower，直到follower跟上为止。Follower中固定一个协程来进行超时选举。所有Server中都有一个固定的协程来更新appliedIndex到最新的commitIndex。
这样设计的坏处有：
1. 所有AE的结果都由一个协程接收，不知道是哪次调用的结果，因此需要在Rpc调用的结果中，记录调用者。
2. 一个Server上可以同时发送多个AE RPC调用，并发问题难以解决，也会造成RPC调用次数过多。比如AppendEntries调用，在没有收到返回结果前，Leader在心跳到期或者收到新的log时，又会发送AppendEntries调用，这样，接受者可能会收到多个重复、多余的AppendEntries调用。

这种设计加重了我debug的难度，但是由于积重难返，我还是按照这个结构来写。更加合理的结构应该是每个Server上的AppendEntries只能同时发送一个。这样强制串行化，不仅节约开销，而且代码写起来也很简单，不会出现难懂的并发问题。此外，效率上也没有差别。因为当前一个rpc还没有返回，又来了新的log时，leader由于获取不到发送RPC调用给follower对应的锁，所以一直等待，所以可以感知到新的log。在上一个rpc调用返回并处理完后，直接发送最新的rpc调用。
### Lab 2A
按照论文和guide写就行。
### Lab 2B
1. applyMsg中command的下标与Start函数中返回的index一定要一样
2. TestFailAgree2B测试：某个follower节点a跟其他节点之间的连接被断开，自己的Term不停增高。然后又跟其他节点取得联系。这时候很容易处理不当。因为Leader的Term还是原来的值，假设为1，而a节点的Term可能比较大，比如8。这时候，节点a将拒绝Leader发来的AE。然后重新发起选举。但是，a由于log太落后，又无法当选为Leader。还会将当前Leader转变为follower。然后，选举重新选出一个Leader，Term为9. 
3. TestFailNotAgree2B：5个节点。其中三个节点会断开连接。然后重新加入。这时候，重新加入的3个节点Term高，但是日志落后与没有断开过的两个节点。不过，仍然有可能从中选出leader。在断开节点重新加入之前，另外两个节点不能commit。
4. TestBackUp2B:5个节点。选举后，增加一条会提交的日志，然后把节点分成两个部分：2，3；在两个节点的partion中，有一个Leader，往这个partion中增加50条不会commit的日志。然后把这个partion停掉。开启3个点的partion，增加50条会提交的日志。然后，从这个partion中停掉一个非Leader的节点，假设为节点4。再重新增加50条不会提交的日志。 然后，停掉所有节点，将节点4和2个节点partion连接。**这时候，节点4必定会成为Leader。**因为这三个点的log长度都是51，并且节点4的日志的Term更大，所以更新。所以他可以获得3张票成为Leader。

### Lab 2C
主要是figure8 unreliable这个测试，在终端输入go run test 2C有概率出错，而在Vscode测试工具中，直接单独运行这个测试则基本不会出错，原因不明。
### Lab 2D
需要对以前的代码进行小型重构。加入rf.lastIncludedIndex作为rf.log的index的逻辑起始地址。这里就引入了逻辑地址（下标）和真实地址（下标）。log的下标称为真实地址，commitIndex、nextIndex、lastLogIndex等则称为逻辑地址。逻辑地址转化为真实地址需要先减去rf.lastIncludedIndex。这里要非常仔细，不能漏掉任何一处地方。
Lab 2D的主要流程：
1. Tester（模拟上层用户）每接收10条log，就通过SnapShot方法通知raft进行日志compact，raft就保存一个snapshot，然后把快照点以前的log都删掉
2. 由于Leader也会进行日志的compact，所以，对于过于落后的follower（nextIndex <= Leader.lastIncludedIndex），通过RPC调用发送快照给follower。follower根据快照进行更新。
3. follower收到快照后，通过applyChan发送快照给Tester
4. Tester收到快照后，再把快照通过CondInstallSnapShot方法，把快照发回follower，follower判断，有没有比快照的lastIncludedIndex更大的已提交日志，如果没有，那么同意进行快照保存，Tester（用户）也进行快照保存。


#### bug1
```
=== RUN   TestSnapshotBasic2D
disconnect(0)
disconnect(1)
disconnect(2)
connect(0)
connect(1)
connect(2)
Test (2D): snapshots basic ...
2022/03/28 21:33:04 [Server:1, term:0, state:FOLLOWER, log_len:1] 经过选举超时未收到心跳，发起选举
2022/03/28 21:33:04 [Server:1, term:1, state:CANDIDATE, log_len:1] 转变为CANDIDATE
2022/03/28 21:33:04 [Server:2, term:0, state:FOLLOWER, log_len:1] 转变为FOLLOWER
2022/03/28 21:33:04 [Server:2, term:1, state:FOLLOWER, log_len:1] 同意来自 [Server:1, term:1, log_len:1] 的投票请求
2022/03/28 21:33:04 [Server:0, term:0, state:FOLLOWER, log_len:1] 转变为FOLLOWER
2022/03/28 21:33:04 [Server:1, term:1, state:LEADER, log_len:1] 被选举为LEADER
2022/03/28 21:33:04 [Server:0, term:1, state:FOLLOWER, log_len:1] 同意来自 [Server:1, term:1, log_len:1] 的投票请求
2022/03/28 21:33:04 [Server:1, term:1, state:LEADER, log_len:2]取得新日志:[Command: 1500657772148685224, index: 1]
2022/03/28 21:33:04 [Server:1, term:1, state:LEADER, log_len:2] 准备向Tester提供Command[commandIndex:1, command:1500657772148685224]
2022/03/28 21:33:04 [Server:1, term:1, state:LEADER, log_len:2] 向Tester提供了Command[commandIndex:1, command:1500657772148685224]
2022/03/28 21:33:04 [Server:0, term:1, state:FOLLOWER, log_len:2] 准备向Tester提供Command[commandIndex:1, command:1500657772148685224]
2022/03/28 21:33:04 [Server:2, term:1, state:FOLLOWER, log_len:2] 准备向Tester提供Command[commandIndex:1, command:1500657772148685224]
2022/03/28 21:33:04 [Server:0, term:1, state:FOLLOWER, log_len:2] 向Tester提供了Command[commandIndex:1, command:1500657772148685224]
2022/03/28 21:33:04 [Server:2, term:1, state:FOLLOWER, log_len:2] 向Tester提供了Command[commandIndex:1, command:1500657772148685224]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:3]取得新日志:[Command: 3872008232781412091, index: 2]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:4]取得新日志:[Command: 655993975682991257, index: 3]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:5]取得新日志:[Command: 3332393325322579636, index: 4]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:6]取得新日志:[Command: 3080134005304087243, index: 5]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:7]取得新日志:[Command: 3380297986469684197, index: 6]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:8]取得新日志:[Command: 2466321549470130527, index: 7]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:9]取得新日志:[Command: 8000768180499482027, index: 8]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:10]取得新日志:[Command: 4767942098991386319, index: 9]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:11]取得新日志:[Command: 3272521778673241473, index: 10]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:12]取得新日志:[Command: 7729447319423661482, index: 11]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:13]取得新日志:[Command: 5188662191884397601, index: 12]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14]取得新日志:[Command: 6355601169747912440, index: 13]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 准备向Tester提供Command[commandIndex:2, command:3872008232781412091]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 向Tester提供了Command[commandIndex:2, command:3872008232781412091]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 准备向Tester提供Command[commandIndex:3, command:655993975682991257]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 向Tester提供了Command[commandIndex:3, command:655993975682991257]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 准备向Tester提供Command[commandIndex:4, command:3332393325322579636]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 向Tester提供了Command[commandIndex:4, command:3332393325322579636]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 准备向Tester提供Command[commandIndex:5, command:3080134005304087243]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 向Tester提供了Command[commandIndex:5, command:3080134005304087243]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 准备向Tester提供Command[commandIndex:6, command:3380297986469684197]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 向Tester提供了Command[commandIndex:6, command:3380297986469684197]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 准备向Tester提供Command[commandIndex:7, command:2466321549470130527]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 向Tester提供了Command[commandIndex:7, command:2466321549470130527]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 准备向Tester提供Command[commandIndex:8, command:8000768180499482027]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 向Tester提供了Command[commandIndex:8, command:8000768180499482027]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 准备向Tester提供Command[commandIndex:9, command:4767942098991386319]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 向Tester提供了Command[commandIndex:9, command:4767942098991386319]
2022/03/28 21:33:05 [Server:1, term:1, state:LEADER, log_len:14] 准备向Tester提供Command[commandIndex:10, command:3272521778673241473]
2022/03/28 21:33:05 [Server:0, term:1, state:FOLLOWER, log_len:14] 经过选举超时未收到心跳，发起选举
2022/03/28 21:33:05 [Server:0, term:2, state:CANDIDATE, log_len:14] 转变为CANDIDATE
2022/03/28 21:33:05 [Server:2, term:1, state:FOLLOWER, log_len:14] 转变为FOLLOWER
2022/03/28 21:33:05 [Server:2, term:2, state:FOLLOWER, log_len:14] 同意来自 [Server:0, term:2, log_len:14] 的投票请求
2022/03/28 21:33:05 [Server:0, term:2, state:LEADER, log_len:14] 被选举为LEADER
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15]取得新日志:[Command: 6355601169747912440, index: 14]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 准备向Tester提供Command[commandIndex:2, command:3872008232781412091]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 向Tester提供了Command[commandIndex:2, command:3872008232781412091]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 准备向Tester提供Command[commandIndex:3, command:655993975682991257]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 向Tester提供了Command[commandIndex:3, command:655993975682991257]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 准备向Tester提供Command[commandIndex:4, command:3332393325322579636]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 向Tester提供了Command[commandIndex:4, command:3332393325322579636]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 准备向Tester提供Command[commandIndex:5, command:3080134005304087243]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 向Tester提供了Command[commandIndex:5, command:3080134005304087243]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 准备向Tester提供Command[commandIndex:6, command:3380297986469684197]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 向Tester提供了Command[commandIndex:6, command:3380297986469684197]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 准备向Tester提供Command[commandIndex:7, command:2466321549470130527]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 向Tester提供了Command[commandIndex:7, command:2466321549470130527]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 准备向Tester提供Command[commandIndex:8, command:8000768180499482027]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 向Tester提供了Command[commandIndex:8, command:8000768180499482027]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 准备向Tester提供Command[commandIndex:9, command:4767942098991386319]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 向Tester提供了Command[commandIndex:9, command:4767942098991386319]
2022/03/28 21:33:07 [Server:0, term:2, state:LEADER, log_len:15] 准备向Tester提供Command[commandIndex:10, command:3272521778673241473]
2022/03/28 21:33:07 [Server:2, term:2, state:FOLLOWER, log_len:15] 经过选举超时未收到心跳，发起选举
2022/03/28 21:33:07 [Server:2, term:3, state:CANDIDATE, log_len:15] 转变为CANDIDATE
2022/03/28 21:33:07 [Server:2, term:3, state:CANDIDATE, log_len:15] 经过选举超时未收到心跳，发起选举
2022/03/28 21:33:07 [Server:2, term:4, state:CANDIDATE, log_len:15] 转变为CANDIDATE
2022/03/28 21:33:07 [Server:2, term:4, state:CANDIDATE, log_len:15] 经过选举超时未收到心跳，发起选举
2022/03/28 21:33:07 [Server:2, term:5, state:CANDIDATE, log_len:15] 转变为CANDIDATE
2022/03/28 21:33:08 [Server:2, term:5, state:CANDIDATE, log_len:15] 经过选举超时未收到心跳，发起选举
2022/03/28 21:33:08 [Server:2, term:6, state:CANDIDATE, log_len:15] 转变为CANDIDATE
```
可以看见，每当发送第9条log给Tester（相当于Service）时，Leader卡死掉。
原因在于config.go中如下语句：
```go
			if (m.CommandIndex+1)%SnapShotInterval == 0 {
				w := new(bytes.Buffer)
				e := labgob.NewEncoder(w)
				v := m.Command
				e.Encode(v)
				cfg.rafts[i].Snapshot(m.CommandIndex, w.Bytes())
			}
```
它检查第10条log。然后调用raft的Snapshot。而在我的实现中，在applyChan<-ApplyMsg处偷懒，没有释放锁。而在Snapshot中又需要进行加锁，所以导致了死锁。再次说明，在进行RPC调用时，要释放锁。

## LAB 3
