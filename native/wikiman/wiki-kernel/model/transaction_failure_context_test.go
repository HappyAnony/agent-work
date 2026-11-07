// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"testing"
)

func TestTransactionFailureUsesCapturedEnvironmentReporter(t *testing.T) {
	want := &TxErr{code: TxErrCodeDataIsSyncing, msg: "fixture"}
	var gotFailure *TxErr
	var gotTransaction *Transaction
	environment := &transactionEnvironment{reportFailure: func(failure *TxErr, tx *Transaction) {
		gotFailure, gotTransaction = failure, tx
	}}
	tx := &Transaction{environment: environment}
	reportTransactionFailure(want, tx)
	if gotFailure != want || gotTransaction != tx {
		t.Fatal("transaction failure bypassed its captured environment")
	}
}

func TestTransactionWorkerReportsThroughCapturedEnvironmentWithoutGlobalConf(t *testing.T) {
	old := Conf
	Conf = nil
	defer func() { Conf = old }()
	called := false
	tx := &Transaction{
		environment: &transactionEnvironment{
			ctx: context.Background(),
			reportFailure: func(failure *TxErr, got *Transaction) {
				called = failure != nil && got != nil
			},
		},
		DoOperations: []*Operation{{Action: "fixture-unbound"}},
	}
	flushTx(tx)
	if !called {
		t.Fatal("transaction worker did not use captured failure reporter")
	}
}
