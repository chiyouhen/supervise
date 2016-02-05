//  Copyright 2015, 2016 ZHANG Heng (chiyouhen@gmail.com)
//
//  This file is part of Supervise.
//  
//  Supervise is free software: you can redistribute it and/or modify
//  it under the terms of the GNU General Public License as published by
//  the Free Software Foundation, either version 3 of the License, or
//  any later version.
//  
//  Supervise is distributed in the hope that it will be useful,
//  but WITHOUT ANY WARRANTY; without even the implied warranty of
//  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//  GNU General Public License for more details.
//  
//  You should have received a copy of the GNU General Public License
//  along with Supervise.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
    "os"
    "os/exec"
    "os/signal"
    "path/filepath"
    "fmt"
    "syscall"
    "io/ioutil"
    "strings"
    getopt "github.com/kesselborn/go-getopt"
)

const (
    SU_OK = 0
    SU_RUNNING = 100
)

type Supervise struct {
    exec string
    prefix string
    cwd string
    rundir string
    ctrlPipePath string
    lockPath string
    lockFd int
    daemon bool
    pidPath string
    statusPath string
    ctrlPipeFd int
    ctrlChan chan byte
    cmd string
    pid int
    action string
    status string
    background bool
    sigChan chan os.Signal
}

func (su *Supervise) WriteLog(f string, v... interface{}) {
    fmt.Fprintf(os.Stderr, f + "\n", v...)
}

func (su *Supervise) FlushStatus() {
    ioutil.WriteFile(su.statusPath, []byte(fmt.Sprintf("%d\n", su.pid)), 0644)
}

func (su *Supervise) BuryChild() {
    var exitcode syscall.WaitStatus
    var rusage = syscall.Rusage{}
    syscall.Wait4(su.pid, &exitcode, syscall.WNOHANG, &rusage)
}

func (su *Supervise) Spawn() {
    var args = make([]string, 3)
    args[0] = "/bin/bash"
    args[1] = "-c"
    args[2] = su.cmd
    var sysProcAttr = syscall.SysProcAttr {}
    sysProcAttr.Setsid = true
    var procAttr = syscall.ProcAttr {
        su.cwd,
        os.Environ(),
        nil,
        &sysProcAttr,
    }
    var pid, _ = syscall.ForkExec("/bin/bash", args, &procAttr)
    su.pid = pid
    su.FlushStatus()
    su.WriteLog("child %d started", su.pid)
}

func (su *Supervise) StartSignalChan() {
    signal.Notify(su.sigChan, syscall.SIGCHLD, syscall.SIGTERM, syscall.SIGINT)
}

func (su *Supervise) StartControlChan() {
    go func() {
        var b = make([]byte, 1)
        var n int
        for {
            n, _ = syscall.Read(su.ctrlPipeFd, b)
            if n > 0 {
                su.ctrlChan<- b[0]
            }
        }
    }()
}

func (su *Supervise) KillProc() {
    var pgid, err = syscall.Getpgid(su.pid)
    if err == nil {
        syscall.Kill(-pgid, syscall.SIGKILL)
    }
}

func (su *Supervise) SuperviseLoop() {
    var c byte
    var s os.Signal
    for {
        select {
            case c = <-su.ctrlChan:
                switch c {
                    case 'k':
                        su.KillProc()
                    case 'x':
                        su.status = "quit"
                        su.KillProc()
                    case 'd', 'p':
                        su.status = "pause"
                        su.KillProc()
                    case 'u':
                        if su.status != "running" {
                            su.status = "running"
                            su.Spawn()
                        }
                }
            case s = <-su.sigChan:
                su.WriteLog("sigChan got '%v'", s)
                switch s {
                    case syscall.SIGCHLD:
                        su.WriteLog("got SIGCHLD")
                        su.BuryChild()
                        switch su.status {
                            case "running":
                                su.Spawn()
                            case "quit":
                                os.Exit(SU_OK)
                        }
                    case syscall.SIGTERM, syscall.SIGINT:
                        su.status = "quit"
                        su.KillProc()
                }
        }
    }
}

func (su *Supervise) Start() {
    if su.AcquireLock() != nil {
        su.WriteLog("process is running.")
        os.Exit(SU_RUNNING)
    }
    su.Daemon()
    su.StartSignalChan()
    su.StartControlChan()
    su.status = "running"
    su.Spawn()
    su.FlushStatus()
    su.SuperviseLoop()
}

func (su *Supervise) Stop() {
    if su.AcquireLock() == nil {
        su.WriteLog("process is not running.")
    } else {
        syscall.Write(su.ctrlPipeFd, []byte("x"))
    }
}

func (su *Supervise) Pause() {
    if su.AcquireLock() == nil {
        su.WriteLog("process is not running.")
    } else {
        syscall.Write(su.ctrlPipeFd, []byte("p"))
    }
}

func (su *Supervise) ExeAbsPath() string {
    if strings.HasPrefix(os.Args[0], "/") {
        return os.Args[0]
    } else if strings.Contains(os.Args[0], "/") {
        cwd, err := os.Getwd()
        if err != nil {
            return os.Args[0]
        }
        return filepath.Join(cwd, os.Args[0])
    } else {
        p, err := exec.LookPath(os.Args[0])
        if err != nil {
            return os.Args[0]
        }
        return p
    }
}

func (su *Supervise) Init() {
    su.exec = su.ExeAbsPath()
    var bindir = filepath.Dir(su.exec)
    su.prefix = filepath.Dir(bindir)
    su.cwd, _ = os.Getwd()
    su.rundir = filepath.Join(su.prefix, "run")
}

func (su *Supervise) CheckEnv() {
    var isBg = os.Getenv("SU_BACKGROUND")
    if isBg == "1" {
        su.background = true
    } else {
        su.background = false
    }
}

func (su *Supervise) FlushPid() {
    ioutil.WriteFile(su.pidPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644)
}

func (su *Supervise) Daemon() {
    if su.background {
        su.FlushPid()
        return
    } else if su.daemon {
        os.Setenv("SU_BACKGROUND", "1")
        var procAttr = syscall.ProcAttr {
            su.cwd,
            os.Environ(),
            nil,
            nil,
        }
        syscall.ForkExec(su.exec, os.Args, &procAttr)
        os.Exit(SU_OK)
    }
}

func (su *Supervise) ParseArgs() {
    var optDef = getopt.Options {
        "supervise",
        getopt.Definitions {
            {"status-dir|d|SU_STATUS_DIR", "status directory", getopt.Required,                           su.rundir},
            {"cmd|c|SU_CMD",               "command line",     getopt.Required,                           "/sbin/hello -d /etc/hello.conf"},
            {"control|s",                  "operation",        getopt.Optional | getopt.ExampleIsDefault, "start"},
            {"daemon|D",                   "run as daemon",    getopt.Flag,                               false},
        },
    }
    var opts, _, _, err = optDef.ParseCommandLine()
    var help, ok = opts["help"]
    if err != nil || ok {
        switch help.String {
            case "help":
                fmt.Print(optDef.Help())
            case "usage":
                fmt.Print(optDef.Usage())
            default:
                fmt.Print(optDef.Help())
        }
        os.Exit(SU_OK)
    }
    for k, v := range opts {
        switch k {
            case "status-dir":
                su.rundir = v.String
                os.Setenv("SU_STATUS_DIR", v.String)
            case "cmd":
                su.cmd = v.String
                os.Setenv("SU_CMD", v.String)
            case "control":
                su.action = v.String
            case "daemon":
                su.daemon = v.Bool
        }
    }
}

func (su *Supervise) OpenControlPipe() {
    syscall.Mkfifo(su.ctrlPipePath, 0600)
    su.ctrlPipeFd, _ = syscall.Open(su.ctrlPipePath, syscall.O_RDWR, 0600)
    syscall.CloseOnExec(su.ctrlPipeFd)
}

func (su *Supervise) Config() {
    su.lockPath = filepath.Join(su.rundir, "supervise.lock")
    su.ctrlPipePath = filepath.Join(su.rundir, "control")
    su.pidPath = filepath.Join(su.rundir, "supervise.pid")
    su.statusPath = filepath.Join(su.rundir, "status")
    su.MkRunDir()
    su.OpenLock()
    su.OpenControlPipe()
    su.sigChan = make(chan os.Signal, 1)
    su.ctrlChan = make(chan byte, 1)
    su.status = "killed"
}

func (su *Supervise) MkRunDir() {
    os.MkdirAll(su.rundir, 0755)
}

func (su *Supervise) OpenLock() {
    su.lockFd, _ = syscall.Open(su.lockPath, syscall.O_APPEND | syscall.O_CREAT, 0644)
    syscall.CloseOnExec(su.lockFd)
}

func (su *Supervise) AcquireLock() error {
    return syscall.Flock(su.lockFd, syscall.LOCK_EX | syscall.LOCK_NB)
}

func (su *Supervise) ReleaseLock() {
    syscall.Flock(su.lockFd, syscall.LOCK_UN)
}

func (su *Supervise) Main() {
    su.Init()
    su.CheckEnv()
    su.ParseArgs()
    su.Config()
    switch su.action {
        case "start":
            su.Start()
        case "stop":
            su.Stop()
        case "pause":
            su.Pause()
    }
}

func main() {
    var app = &Supervise{}
    app.Main()
}
