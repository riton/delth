package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
)

type executor struct {
	cmd            *exec.Cmd
	log            *slog.Logger
	ctx            context.Context
	cmdWaitErr     chan error
	onCmdFailureCb func(*exec.ExitError)
}

func NewCmdExecutor(ctx context.Context, name string, args ...string) *executor {
	e := &executor{
		cmd:        exec.Command(name, args...),
		log:        slog.Default().With("component", "executor"),
		ctx:        ctx,
		cmdWaitErr: make(chan error, 1),
	}
	e.cmd.Stdin = os.Stdin
	e.cmd.Stderr = os.Stderr
	e.cmd.Stdout = os.Stdout
	e.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // stop signal propagation
	}

	return e
}

func (e *executor) SetOnCmdFailureCb(cb func(*exec.ExitError)) {
	e.onCmdFailureCb = cb
}

func (e *executor) Start() error {
	if err := e.cmd.Start(); err != nil {
		return err
	}

	e.log.Debug("command successfully started")

	go func() {
		err := e.cmd.Wait()
		ctxErr := e.ctx.Err()
		ignoreCmdFailures := ctxErr != nil || ctxErr == context.Canceled
		e.log.Debug("command exited", "error", err, "ignore-cmd-errors", ignoreCmdFailures)

		if !ignoreCmdFailures {
			if eerr, ok := err.(*exec.ExitError); ok {
				if eerr.ExitCode() != 0 {
					e.log.Error("command error", "exit-code", eerr.ExitCode())
					if e.onCmdFailureCb != nil {
						e.onCmdFailureCb(eerr)
					}
				}
			}
		}

		e.cmdWaitErr <- err
	}()

	return nil
}

// TODO: Allow to send a customized signal
func (e *executor) Stop() error {
	if e.cmd.Process == nil {
		return fmt.Errorf("empty process")
	}

	e.log.Debug("signaling underlying process")

	if err := e.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signaling underlying process: %w", err)
	}

	e.log.Debug("waiting for underlying process")

	if err := <-e.cmdWaitErr; err != nil {
		return fmt.Errorf("waiting for underlying process: %w", err)
	}

	e.log.Debug("underlying process has exited")

	return nil
}
