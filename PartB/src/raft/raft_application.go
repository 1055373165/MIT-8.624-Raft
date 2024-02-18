package raft

func (rf *Raft) applyTicker() {
	for !rf.killed() {
		rf.mu.Lock()
		// wait for applyCond
		rf.applyCond.Wait()
		// 1. collect all entries needed to be applied
		entries := make([]LogEntry, 0)
		for i := rf.lastApplied + 1; i <= rf.commitIndex; i++ {
			entries = append(entries, rf.log[i])
		}
		rf.mu.Unlock()

		// 2. apply entries to state machine
		for i, entry := range entries {
			rf.applyCh <- ApplyMsg{
				CommandValid: entry.CommandValid,
				Command:      entry.Command,
				CommandIndex: rf.lastApplied + 1 + i,
			}
		}

		// 3. update lastApplied
		rf.mu.Lock()
		LOG(rf.me, rf.currentTerm, DApply, "Apply log for [%d, %d]", rf.lastApplied+1, rf.lastApplied+len(entries))
		rf.lastApplied += len(entries)
		rf.mu.Unlock()
	}
}
