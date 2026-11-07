package model

import (
	"context"
	"testing"

	"wiki-kernel/conf"
)

func TestLegacyHistoryExplicitStartStopRestart(t *testing.T) {
	previous := Conf
	defer func() { Conf = previous }()
	Conf = NewAppConf()
	Conf.Editor = &conf.Editor{GenerateHistoryInterval: 3600}
	if historyTicker != nil || historyOwner != nil {
		t.Fatal("history started at import")
	}
	AutoGenerateFileHistory()
	first := historyOwner
	AutoGenerateFileHistory()
	if historyOwner != first {
		t.Fatal("duplicate owner")
	}
	ChangeHistoryTick(3600)
	StopAutoGenerateFileHistory()
	if err := DrainAutoGenerateFileHistoryContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if historyTicker != nil {
		t.Fatal("ticker retained")
	}
	AutoGenerateFileHistory()
	if historyOwner == first {
		t.Fatal("did not reopen after drain")
	}
	StopAutoGenerateFileHistory()
	DrainAutoGenerateFileHistory()
	historyOwner = nil
}
