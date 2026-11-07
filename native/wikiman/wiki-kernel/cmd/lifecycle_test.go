package cmd

import "testing"

type blockingCommand struct{ entered, release chan struct{} }

func (*blockingCommand) Name() string { return "test" }
func (*blockingCommand) IsRead() bool { return true }
func (*blockingCommand) Id() float64  { return 0 }
func (c *blockingCommand) Exec()      { close(c.entered); <-c.release }

func TestExecJoinsActualCommand(t *testing.T) {
	c := &blockingCommand{make(chan struct{}), make(chan struct{})}
	done := make(chan struct{})
	go func() { Exec(c); close(done) }()
	<-c.entered
	select {
	case <-done:
		t.Fatal("command detached from callback")
	default:
	}
	close(c.release)
	<-done
}
