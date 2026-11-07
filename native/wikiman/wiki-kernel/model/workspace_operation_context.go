package model

import (
	"context"
	"errors"
	"wiki-kernel/conf"
)

// Admission and lifetime are shared by compatibility operations. Native calls
// use their acquired Lease instead; this binding never switches global roots.
func (o *legacyReindexOwner) run(ctx context.Context, fn func(context.Context, context.Context) error) error {
	return o.runFinalized(ctx, fn, nil)
}

// Finalization runs after the last operation guard and before the owner's task
// completes, including cancellation/panic. It cannot be detached by shutdown.
func (o *legacyReindexOwner) runFinalized(ctx context.Context, fn func(context.Context, context.Context) error, finalize func(error) error) error {
	if ctx == nil || fn == nil {
		return conf.ErrBusinessUnbound
	}
	if err := o.guard(ctx); err != nil {
		return err
	}
	done := make(chan error, 1)
	if !o.owner.startRepoBackground(func(life context.Context) {
		life = context.WithValue(life, legacyAdmissionKey{}, o)
		op, cancel := context.WithCancel(context.WithValue(ctx, legacyAdmissionKey{}, o))
		stop := context.AfterFunc(life, cancel)
		var err error
		defer func() {
			if recover() != nil {
				err = errors.New("workspace_operation_handler_panic")
			}
			if finalize != nil {
				func() {
					defer func() {
						if recover() != nil {
							err = errors.Join(err, errors.New("workspace_operation_cleanup_panic"))
						}
					}()
					err = errors.Join(err, finalize(err))
				}()
			}
			stop()
			cancel()
			done <- err
		}()
		if err = o.guard(op); err == nil {
			err = fn(op, life)
		}
		if err == nil {
			err = o.guard(op)
		}
	}) {
		return conf.ErrBusinessUnbound
	}
	return <-done
}
