// agentinfo writes the AgentInfo of the agents `make agent` builds: jailwick's version, the build
// date and the changes to the agent's code, from git. Run by `make agent`; the file is git-ignored.
package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/buildinfo"
)

const module = "github.com/jailwick/jailwick"

// maxChanges is how many of the agent's latest changes a build lists.
const maxChanges = 30

func main() {
	log.SetFlags(0)
	log.SetPrefix("agentinfo: ")
	if len(os.Args) != 2 {
		log.Fatal("usage: agentinfo <out.json>")
	}
	info := assets.AgentInfo{Release: buildinfo.Version, Built: time.Now().Format(time.DateOnly), Changes: []assets.AgentChange{}}
	dirs, err := agentDirs()
	if err != nil {
		log.Fatal(err)
	}
	if status, err := git(append([]string{"status", "--porcelain", "--"}, dirs...)...); err == nil && strings.TrimSpace(status) != "" {
		info.Changes = append(info.Changes, assets.AgentChange{Date: info.Built, Text: "Changes not committed yet"})
	}
	if out, err := git(append([]string{"log", "-n", strconv.Itoa(maxChanges), "--format=%h%x00%cs%x00%s", "--"}, dirs...)...); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if f := strings.SplitN(line, "\x00", 3); len(f) == 3 {
				info.Changes = append(info.Changes, assets.AgentChange{Commit: f[0], Date: f[1], Text: assets.ChangeText(f[2])})
			}
		}
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(os.Args[1], append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
}

// agentDirs are the folders of this module the agent is built from, relative to the repo.
func agentDirs() ([]string, error) {
	out, err := exec.Command("go", "list", "-deps", "-f",
		`{{if .Module}}{{if eq .Module.Path "`+module+`"}}{{.Dir}}{{end}}{{end}}`, "./cmd/jailwick-agent").Output()
	if err != nil {
		return nil, err
	}
	wd, _ := os.Getwd()
	var dirs []string
	for _, d := range strings.Fields(string(out)) {
		dirs = append(dirs, strings.TrimPrefix(strings.TrimPrefix(d, wd), "/"))
	}
	return dirs, nil
}

func git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return string(out), err
}
