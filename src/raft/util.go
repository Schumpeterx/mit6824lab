package raft

import "log"

// Debugging
const Debug = false        // elect
const TestB = false        // AE
const Tester = false       // applyMsg
const Backup2BTest = false // 测试TestBackup2B
const Lab2D = false
const EntrantOut = false // 在每个函数的进入和出口处打印
func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

func BPrintf(format string, a ...interface{}) (n int, err error) {
	if TestB {
		log.Printf(format, a...)
	}
	return
}

func TesterPrintf(format string, a ...interface{}) (n int, err error) {
	if Tester {
		log.Printf(format, a...)
	}
	return
}
func Backup2BTestPrintf(format string, a ...interface{}) (n int, err error) {
	if Backup2BTest {
		log.Printf(format, a...)
	}
	return
}
func Lab2DPrintf(format string, a ...interface{}) (n int, err error) {
	if Lab2D {
		log.Printf(format, a...)
	}
	return
}
func EntrantOutPrintf(format string, a ...interface{}) (n int, err error) {
	if EntrantOut {
		log.Printf(format, a...)
	}
	return
}
