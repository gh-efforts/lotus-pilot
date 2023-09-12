package miner

import (
	"github.com/filecoin-project/go-address"
	"github.com/google/uuid"
)

type SwitchID uuid.UUID

func (s SwitchID) String() string {
	return uuid.UUID(s).String()
}

type switching struct {
	id    SwitchID
	from  address.Address
	to    address.Address
	count int64
}

func (m *Miner) run() {
	go func() {
		for {
			select {
			case sw := <-m.ch:
				go m.process(sw)
			case <-m.ctx.Done():
				return
			}
		}
	}()
}

func (m *Miner) process(sw switching) error {
	m.swLk.Lock()
	m.switchs[sw.id] = sw
	m.swLk.Unlock()

	m.workerSort(sw)

	return nil
}
