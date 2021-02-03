// gionice is a port of ionice to Go

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/jessevdk/go-flags"
	"github.com/xyproto/ionice"
)

type Options struct {
	Class     string `short:"c" long:"class" description:"name or number of scheduling class, 0: none, 1: realtime, 2: best-effort, 3: idle" choice:"0" choice:"1" choice:"2" choice:"3" choice:"none" choice:"realtime" choice:"best-effort" choice:"idle"`
	ClassData int    `short:"n" long:"classdata" description:"priority (0..7) in the specified scheduling class, only for the realtime and best-effort classes" choice:"0" choice:"1" choice:"2" choice:"3" choice:"4" choice:"5" choice:"6" choice:"7" choice:"8" choice:"9"`
	PID       int    `short:"p" long:"pid" description:"act on these already running processes" value-name:"PID"`
	PGID      int    `short:"P" long:"pgid" description:"act on already running processes in these groups" value-name:"PGID"`
	Ignore    bool   `short:"t" long:"ignore" description:"ignore failures"`
	UID       int    `short:"u" long:"uid" description:"act on already running processes owned by these users" value-name:"UID"`
	Help      bool   `short:"h" long:"help" description:"display this help"`
	Version   bool   `short:"V" long:"version" description:"display version"`
	Args      struct {
		Command []string
	} //`positional-args:"yes" required:"yes"`
}

const versionString = "gionice 1.0.0"

const usageString = `Usage:
 gionice [options] -p <pid>...
 gionice [options] -P <pgid>...
 gionice [options] -u <uid>...
 gionice [options] <command>

Show or change the I/O-scheduling class and priority of a process.

Options:
 -c, --class <class>    name or number of scheduling class,
                          0: none, 1: realtime, 2: best-effort, 3: idle
 -n, --classdata <num>  priority (0..7) in the specified scheduling class,
                          only for the realtime and best-effort classes
 -p, --pid <pid>...     act on these already running processes
 -P, --pgid <pgrp>...   act on already running processes in these groups
 -t, --ignore           ignore failures
 -u, --uid <uid>...     act on already running processes owned by these users

 -h, --help             display this help
 -V, --version          display version

For more details see gionice(1).`

func main() {
	opts := &Options{}
	parser := flags.NewParser(opts, flags.PassAfterNonOption|flags.PassDoubleDash)
	args, err := parser.Parse()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	hasClass := parser.FindOptionByLongName("class").IsSet()
	hasClassData := parser.FindOptionByLongName("classdata").IsSet()
	hasPID := parser.FindOptionByLongName("pid").IsSet()
	hasPGID := parser.FindOptionByLongName("pgid").IsSet()
	hasUID := parser.FindOptionByLongName("uid").IsSet()

	var (
		data                         = 4
		set                          = 0
		ioclass  ionice.IOPRIO_CLASS = ionice.IOPRIO_CLASS_BE
		which                        = 0
		who                          = 0
		tolerant bool
	)

	switch {
	case hasClassData:
		set |= 1
	case hasClass:
		ioclass, err = ionice.Parse(opts.Class)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if ioclass < 0 {
			fmt.Fprintf(os.Stderr, "uknown scheduling class: '%s'\n", opts.Class)
		}
		set |= 2
	case hasPID:
		if who != 0 {
			fmt.Fprintln(os.Stderr, "can handle only one of pid, pgid or uid at once")
			os.Exit(1)
		}
		which = opts.PID
		who = ionice.IOPRIO_WHO_PROCESS
	case hasPGID:
		if who != 0 {
			fmt.Fprintln(os.Stderr, "can handle only one of pid, pgid or uid at once")
			os.Exit(1)
		}
		which = opts.PGID
		who = ionice.IOPRIO_WHO_PGRP
	case hasUID:
		if who != 0 {
			fmt.Fprintln(os.Stderr, "can handle only one of pid, pgid or uid at once")
			os.Exit(1)
		}
		which = opts.UID
		who = ionice.IOPRIO_WHO_USER
	case opts.Ignore:
		tolerant = true
	case opts.Version:
		fmt.Println(versionString)
		os.Exit(0)
	case opts.Help:
		fmt.Println(usageString)
		os.Exit(0)
	}
	switch ioclass {
	case ionice.IOPRIO_CLASS_NONE:
		if (set&1) != 0 && !tolerant {
			// warning
			fmt.Fprintln(os.Stderr, "ignoring given cass data for none class")
		}
		data = 0
	case ionice.IOPRIO_CLASS_RT, ionice.IOPRIO_CLASS_BE:
		break
	case ionice.IOPRIO_CLASS_IDLE:
		if (set&1) != 0 && !tolerant {
			// warning
			fmt.Fprintln(os.Stderr, "ignoring given class data for idle class")
		}
		data = 7
	default:
		if !tolerant {
			// warning
			fmt.Fprintf(os.Stderr, "unknown prio class %d\n", ioclass)
		}
	}

	if set == 0 && which == 0 && len(args) == 0 {
		// gionice without options, print the current ioprio
		ionice.Print(0, ionice.IOPRIO_WHO_PROCESS)
	} else if set == 0 && who != 0 {
		// gionice -p|-P|-u ID [ID ...]
		ionice.Print(which, who)
		for _, id := range args {
			if n, err := strconv.Atoi(id); err == nil { // success, arg is a number
				which = n
				ionice.Print(which, who)
			}
		}

	} else if set != 0 && who != 0 {
		// gionice -c CLASS -p|-P|-u ID [ID ...]
		ionice.SetIDPri(which, ioclass, data, who, tolerant)
		for _, id := range args {
			if n, err := strconv.Atoi(id); err == nil { // success, arg is a number
				which = n
				ionice.SetIDPri(which, ioclass, data, who, tolerant)
			}
		}
	} else if len(args) > 0 {
		// gionice [-c CLASS] COMMAND
		ionice.SetIDPri(0, ioclass, data, ionice.IOPRIO_WHO_PROCESS, tolerant)
		var argv0 string = args[0] // got to find the path first?
		argv0, err := exec.LookPath(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not find %s in PATH\n", args[0])
			os.Exit(1)
		}
		var argv = args
		var envv []string = []string{}
		err = syscall.Exec(argv0, argv, envv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to execute %s\n", argv0)
			os.Exit(1)
		}
		os.Exit(1)
	} else {
		fmt.Fprintln(os.Stderr, "bad usage")
		fmt.Fprintln(os.Stderr, "Try 'gionice --help' for more information.")
		os.Exit(1)
	}
}
