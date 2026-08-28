package service

import "time"

// ptrTime 返回指向给定时间的指针。
//
// 该辅助函数原本住在订阅体系的测试文件里，随订阅拆除一起被删掉，
// 但幂等协调器与系统操作锁的测试仍在用它。这两个测试文件都没有 build tag
// （unit 与 integration 两种编译都会带上），所以这里也不能加 tag。
func ptrTime(t time.Time) *time.Time { return &t }
