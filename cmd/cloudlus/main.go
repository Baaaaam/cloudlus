package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rwcarlsen/cloudlus/cloudlus"
)

var addr = flag.String("addr", "127.0.0.1:9875", "network address of dispatch server")

type CmdFunc func(cmd string, args []string)

var cmds = map[string]CmdFunc{
	"serve":         serve,
	"work":          work,
	"submit":        submit,
	"submit-infile": submitInfile,
	"retrieve":      retrieve,
	"status":        stat,
	"pack":          pack,
	"unpack":        unpack,
}

func newFlagSet(cmd, args, desc string) *flag.FlagSet {
	fs := flag.NewFlagSet("put", flag.ExitOnError)
	fs.Usage = func() {
		log.Printf("Usage: cloudlus %s [OPTION] %s\n%s\n", cmd, args, desc)
		fs.PrintDefaults()
	}
	return fs
}

func main() {
	log.SetFlags(0)
	flag.Usage = func() {
		log.Printf("Usage: cloudlus [OPTION] <subcommand> [OPTION] [args]\n")
		flag.PrintDefaults()
		log.Printf("Subcommands:\n")
		for cmd, _ := range cmds {
			log.Printf("  %v", cmd)
		}
	}
	flag.Parse()

	if len(flag.Args()) == 0 {
		flag.Usage()
		return
	}

	cmd, ok := cmds[flag.Arg(0)]
	if !ok {
		flag.Usage()
		return
	}

	cmd(flag.Arg(0), flag.Args()[1:])
}

func serve(cmd string, args []string) {
	fs := newFlagSet(cmd, "", "run a work dispatch server listening for jobs and workers")
	host := fs.String("host", "", "server host base url")
	fs.Parse(args)
	s := cloudlus.NewServer(*addr)
	s.Host = fulladdr(*host)
	err := s.ListenAndServe()
	fatalif(err)
}

func work(cmd string, args []string) {
	fs := newFlagSet(cmd, "", "run a worker polling for jobs and workers")
	wait := fs.Duration("interval", 10*time.Second, "time interval between work polls when idle")
	fs.Parse(args)
	w := &cloudlus.Worker{ServerAddr: *addr, Wait: *wait}
	w.Run()
}

func submit(cmd string, args []string) {
	fs := newFlagSet(cmd, "[FILE...]", "submit a job file (may be piped to stdin)")
	async := fs.Bool("async", false, "true for asynchronous submission")
	fs.Parse(args)

	data := stdin(fs)
	jobs := []*cloudlus.Job{}
	if data != nil {
		jobs = append(jobs, loadJob(data))
	} else {
		for _, fname := range fs.Args() {
			data, err := ioutil.ReadFile(fname)
			fatalif(err)
			jobs = append(jobs, loadJob(data))
		}
	}

	run(jobs, *async)
}

func submitInfile(cmd string, args []string) {
	fs := newFlagSet(cmd, "[FILE...]", "submit a cyclus input file with default run params (may be piped to stdin)")
	async := fs.Bool("async", false, "true for asynchronous submission")
	fs.Parse(args)

	data := stdin(fs)
	jobs := []*cloudlus.Job{}
	if data != nil {
		jobs = append(jobs, cloudlus.NewJobDefault(data))
	} else {
		for _, fname := range fs.Args() {
			data, err := ioutil.ReadFile(fname)
			fatalif(err)
			jobs = append(jobs, cloudlus.NewJobDefault(data))
		}
	}

	run(jobs, *async)
}

func run(jobs []*cloudlus.Job, async bool) {
	client, err := cloudlus.Dial(*addr)
	fatalif(err)
	defer client.Close()

	ch := make(chan *cloudlus.Job, len(jobs))
	for _, j := range jobs {
		client.Start(j, ch)
		if async {
			fmt.Printf("%x\n", j.Id)
		}
	}

	if !async {
		for _ = range jobs {
			j := <-ch
			if err := client.Err(); err != nil {
				log.Println(err)
				continue
			}

			fname := fmt.Sprintf("result-%x.json", j.Id)
			err := ioutil.WriteFile(fname, saveJob(j), 0755)
			if err != nil {
				log.Println(err)
			} else {
				fmt.Println(fname)
			}
		}
	}
}

func retrieve(cmd string, args []string) {
	fs := newFlagSet(cmd, "[JOBID...]", "retrieve the result tar file for the given job id")
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		log.Fatal("no job id specified")
	}

	client, err := cloudlus.Dial(*addr)
	fatalif(err)
	defer client.Close()

	for _, arg := range fs.Args() {
		uid, err := hex.DecodeString(arg)
		if err != nil {
			log.Println(err)
			continue
		}
		var jid cloudlus.JobId
		copy(jid[:], uid)

		j, err := client.Retrieve(jid)
		if err != nil {
			log.Println(err)
			continue
		}

		fname := fmt.Sprintf("result-%x.json", j.Id)
		err = ioutil.WriteFile(fname, saveJob(j), 0755)
		if err != nil {
			log.Println(err)
		}
	}
}

func stat(cmd string, args []string) {
	fs := newFlagSet(cmd, "JOBID [JOBID...]", "get the status of the given job id")
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		log.Fatal("no job id specified")
	}

	for _, arg := range fs.Args() {
		resp, err := http.Get(fulladdr(*addr) + "/job/status/" + fs.Arg(0))
		fatalif(err)
		data, err := ioutil.ReadAll(resp.Body)
		fatalif(err)

		j := cloudlus.NewJob()
		if err := json.Unmarshal(data, &j); err != nil {
			log.Printf("[ERR] no such job %v\n", arg)
		} else {
			fmt.Printf("%x: %v\n", j.Id, j.Status)
		}
	}
}

func unpack(cmd string, args []string) {
	fs := newFlagSet(cmd, "", "unpack all the named job files' output files into id-named directories")
	fs.Parse(args)

	for _, fname := range fs.Args() {
		data, err := ioutil.ReadFile(fname)
		fatalif(err)
		j := loadJob(data)

		dirname := fmt.Sprintf("outfiles-%x", j.Id)

		err = os.MkdirAll(dirname, 0755)
		fatalif(err)

		for _, f := range j.Outfiles {
			p := filepath.Join(dirname, f.Name)
			err := ioutil.WriteFile(p, f.Data, 0755)
			fatalif(err)
		}
		fmt.Println(dirname)
	}
}

func pack(cmd string, args []string) {
	fs := newFlagSet(cmd, "", "pack all files in the working directory into a job submit file")
	fname := fs.String("o", "", "send pack data to file instead of stdout")
	fs.Parse(args)

	d, err := os.Open(".")
	fatalif(err)
	defer d.Close()

	files, err := d.Readdir(-1)
	fatalif(err)
	j := cloudlus.NewJob()
	for _, info := range files {
		if info.IsDir() {
			continue
		}
		data, err := ioutil.ReadFile(info.Name())
		fatalif(err)
		if info.Name() == "cmd.txt" {
			err := json.Unmarshal(data, &j.Cmd)
			fatalif(err)
		} else if info.Name() == "want.txt" {
			list := []string{}
			err := json.Unmarshal(data, &list)
			fatalif(err)
			for _, name := range list {
				j.AddOutfile(name)
			}
		} else {
			j.AddInfile(info.Name(), data)
		}
	}
	data, err := json.Marshal(j)
	fatalif(err)

	if *fname == "" {
		fmt.Printf("%s\n", data)
	} else {
		err := ioutil.WriteFile(*fname, data, 0644)
		fatalif(err)
	}
}

func fatalif(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func fulladdr(addr string) string {
	if !strings.HasPrefix(addr, "http://") && addr != "" {
		return "http://" + addr
	}
	return addr
}

func stdin(fs *flag.FlagSet) []byte {
	if len(fs.Args()) > 0 {
		return nil
	}
	data, err := ioutil.ReadAll(os.Stdin)
	fatalif(err)
	return data
}

func loadJob(data []byte) *cloudlus.Job {
	j := &cloudlus.Job{}
	err := json.Unmarshal(data, &j)
	fatalif(err)
	return j
}

func saveJob(j *cloudlus.Job) []byte {
	data, err := json.Marshal(j)
	fatalif(err)
	return data
}
