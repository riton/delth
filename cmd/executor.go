/*
Copyright © 2024 Rémi Ferrand

Contributor(s): Rémi Ferrand <riton.github_at_gmail.com>, 2024

This software is governed by the CeCILL license under French law and
abiding by the rules of distribution of free software.  You can  use,
modify and/ or redistribute the software under the terms of the CeCILL
license as circulated by CEA, CNRS and INRIA at the following URL
"http://www.cecill.info".

As a counterpart to the access to the source code and  rights to copy,
modify and redistribute granted by the license, users are provided only
with a limited warranty  and the software's author,  the holder of the
economic rights,  and the successive licensors  have only  limited
liability.

In this respect, the user's attention is drawn to the risks associated
with loading,  using,  modifying and/or developing or reproducing the
software by the user in light of its specific status of free software,
that may mean  that it is complicated to manipulate,  and  that  also
therefore means  that it is reserved for developers  and  experienced
professionals having in-depth computer knowledge. Users are therefore
encouraged to load and test the software's suitability as regards their
requirements in conditions enabling the security of their systems and/or
data to be ensured and,  more generally, to use and operate it in the
same conditions as regards security.

The fact that you are presently reading this means that you have had
knowledge of the CeCILL license and that you accept its terms.
*/
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
