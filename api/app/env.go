// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"fmt"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/log"
	"os"
	"path"
	"time"
)

const (
	chanSize    = 10
	runAttempts = 5
)

type message struct {
	app     *App
	success chan bool
}

var env chan message = make(chan message, chanSize)

var environConfPath = path.Join(os.ExpandEnv("${HOME}"), ".juju", "environments.yaml")

type cmd struct {
	cmd    string
	result chan cmdResult
	u      *Unit
}

type cmdResult struct {
	err    error
	output []byte
}

var cmds chan cmd = make(chan cmd, chanSize)

func init() {
	go collectEnvVars()
	go runCommands()
}

func runCommands() {
	for cmd := range cmds {
		buf := new(bytes.Buffer)
		err := cmd.u.Command(buf, buf, cmd.cmd)
		if cmd.result != nil {
			r := cmdResult{output: buf.Bytes(), err: err}
			cmd.result <- r
		}
	}
}

func runCmd(command string, msg message, databaseTimeout time.Duration) {
	unit := msg.app.unit()
	for unit.Machine == 0 {
		time.Sleep(databaseTimeout)
		err := msg.app.Get()
		if err != nil {
			return
		}
		unit = msg.app.unit()
	}
	c := cmd{
		u:      unit,
		cmd:    command,
		result: make(chan cmdResult),
	}
	cmds <- c
	var r cmdResult
	r = <-c.result
	for i := 0; r.err != nil && i < runAttempts; i++ {
		time.Sleep(1e9)
		cmds <- c
		r = <-c.result
	}
	log.Printf("running %s on %s, output:\n %s", command, msg.app.Name, string(r.output))
	if msg.success != nil {
		msg.success <- r.err == nil
	}
}

func collectEnvVars() {
	for e := range env {
		cmd := "cat > /home/application/apprc <<END\n"
		cmd += fmt.Sprintf("# generated by tsuru at %s\n", time.Now().Format(time.RFC822Z))
		for k, v := range e.app.Env {
			cmd += fmt.Sprintf(`export %s="%s"`+"\n", k, v.Value)
		}
		cmd += "END\n"
		runCmd(cmd, e, 5e9)
	}
}

var fsystem fs.Fs

func filesystem() fs.Fs {
	if fsystem == nil {
		fsystem = fs.OsFs{}
	}
	return fsystem
}
