package f123

import (
	"fmt"
	"sync"
)

// LatestLapData is a map of each (session, user) to the index of the latest full lap that ingest has received and sent
type LatestLapData struct {
	lock sync.RWMutex
	data map[string]int
}

func NewLatestLapData() *LatestLapData {
	return &LatestLapData{
		data: make(map[string]int),
	}
}

func (l *LatestLapData) Set(sessionID, userID string, lapNum int) int {
	l.lock.Lock()
	defer l.lock.Unlock()
	key := fmt.Sprintf("%s.%s", sessionID, userID)
	if latestLapNum := l.data[key]; lapNum > latestLapNum {
		l.data[key] = lapNum
	}
	return l.data[key]
}

func (l *LatestLapData) Get(sessionID, userID string) int {
	l.lock.RLock()
	defer l.lock.RUnlock()
	key := fmt.Sprintf("%s.%s", sessionID, userID)
	return l.data[key]
}
