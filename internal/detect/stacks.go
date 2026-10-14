package detect

import (
	"encoding/json"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/jailwick/jailwick/internal/stacks"
)

// stackSource is a kind of file that shows stacks, and reads their versions from it: stack name
// to version, "" when the file shows the stack but not a version.
type stackSource struct {
	names []string // base names or globs
	read  func(data []byte) map[string]string
}

// stackSources in order of trust: at the same depth, an earlier source's version wins.
var stackSources = []stackSource{
	{[]string{".tool-versions"}, toolVersions},
	{[]string{".nvmrc", ".node-version"}, only("node", nodeVersionFile)},
	{[]string{".python-version"}, only("python", firstLine)},
	{[]string{"rust-toolchain.toml", "rust-toolchain"}, only("rust", rustChannel)},
	{[]string{"go.work", "go.mod"}, only("go", goVersion)},
	{[]string{"package.json"}, packageJSONStacks},
	{[]string{"pyproject.toml"}, only("python", requiresPython)},
	{[]string{"requirements*.txt", "setup.py", "setup.cfg", "Pipfile", "uv.lock", "poetry.lock"}, only("python", nil)},
	{[]string{"Cargo.toml"}, only("rust", nil)},
	{[]string{"bun.lock", "bun.lockb", "bunfig.toml"}, only("bun", nil)},
	{[]string{".java-version"}, only("java", javaMajorLine)},
	{[]string{".sdkmanrc"}, only("java", sdkmanJava)},
	{[]string{"pom.xml", "mvnw", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "gradlew"}, only("java", nil)},
	{[]string{"deps.edn", "project.clj", "build.sbt"}, only("java", nil)}, // Clojure and Scala run on the JVM
	// Ruby is the system's: a version file only shows Ruby (another version is a base image's).
	{[]string{"Gemfile", "Gemfile.lock", "*.gemspec", ".ruby-version"}, only("ruby", nil)},
}

var majorRE = regexp.MustCompile(`\d+(\.\d+)?`)

// javaMajor reads a Java version as its feature release: 21 from "21.0.2-tem", "temurin-21" or
// "openjdk-21.0.1"; 8 from "1.8". "" when there is none.
func javaMajor(v string) string {
	m := majorRE.FindString(v)
	if m == "" {
		return ""
	}
	major, minor, _ := strings.Cut(m, ".")
	if major == "1" && minor != "" {
		return minor
	}
	return major
}

func javaMajorLine(data []byte) string { return javaMajor(firstLine(data)) }

// sdkmanJava reads SDKMAN's .sdkmanrc ("java=21.0.2-tem").
func sdkmanJava(data []byte) string {
	for _, line := range lines(data) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "java="); ok {
			return javaMajor(v)
		}
	}
	return ""
}

// stackFinding is a stack seen in a file at some depth of the repo.
type stackFinding struct {
	depth, source int
	version       string
}

// stackFindings collects, per stack, where the repo shows it.
type stackFindings map[string][]stackFinding

// add reads a file of the repo if it shows stacks.
func (f stackFindings) add(name string, depth int, read func() ([]byte, bool)) {
	for i, src := range stackSources {
		if !matchesAny(src.names, name) {
			continue
		}
		data, ok := read()
		if !ok {
			return
		}
		for stack, v := range src.read(data) {
			f[stack] = append(f[stack], stackFinding{depth: depth, source: i, version: v})
		}
		return
	}
}

// chosen returns each stack the repo shows, with the version of its shallowest, most trusted
// file that names one installable version ("" for the stack's default).
func (f stackFindings) chosen() map[string]string {
	out := map[string]string{}
	for name, found := range f {
		s, _ := stacks.Get(name)
		best, bestAt := "", stackFinding{depth: 1 << 30}
		for _, x := range found {
			if x.version == "" || s.Check(x.version) != nil {
				continue
			}
			if x.depth < bestAt.depth || (x.depth == bestAt.depth && x.source < bestAt.source) {
				best, bestAt = x.version, x
			}
		}
		out[name] = best
	}
	return out
}

func matchesAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// only reads a file that shows one stack; read nil: the file shows it without a version.
func only(stack string, read func([]byte) string) func([]byte) map[string]string {
	return func(data []byte) map[string]string {
		v := ""
		if read != nil {
			v = read(data)
		}
		return map[string]string{stack: v}
	}
}

func firstLine(data []byte) string {
	for _, line := range lines(data) {
		return strings.Fields(line)[0]
	}
	return ""
}

// nodeCodenames are the LTS lines that .nvmrc can name (lts/iron).
var nodeCodenames = map[string]string{"hydrogen": "18", "iron": "20", "jod": "22", "krypton": "24"}

// nodeVersionFile reads .nvmrc or .node-version: "22", "v22.11.0", "lts/iron", "lts/*" (the
// default).
func nodeVersionFile(data []byte) string {
	v := strings.TrimPrefix(firstLine(data), "v")
	if codename, ok := strings.CutPrefix(v, "lts/"); ok {
		return nodeCodenames[strings.ToLower(codename)]
	}
	return nodeMajorOrExact(v)
}

var versionRE = regexp.MustCompile(`\d+(\.\d+)*`)

// nodeMajorOrExact keeps an exact version (22.11.0) and shortens anything else to its major.
func nodeMajorOrExact(v string) string {
	v = versionRE.FindString(v)
	if strings.Count(v, ".") == 2 {
		return v
	}
	major, _, _ := strings.Cut(v, ".")
	return major
}

// nodeRange reads an engines range: a bare version ("22.11.0") as is, any other range ("^20.10.0",
// "20.x") as its major.
func nodeRange(r string) string {
	r = strings.TrimSpace(r)
	if versionRE.FindString(r) == r {
		return nodeMajorOrExact(r)
	}
	major, _, _ := strings.Cut(versionRE.FindString(r), ".")
	return major
}

// openRange reports whether a version range has no upper bound (">=20"): any newer version fits,
// so the default is as good as its lower bound.
func openRange(r string) bool {
	r = strings.TrimSpace(r)
	return strings.HasPrefix(r, ">") && !strings.Contains(r, "<")
}

// atLeast returns "" (the default) when the stack's default is at least min, else min.
func atLeast(stack, min string) string {
	s, _ := stacks.Get(stack)
	if compareVersions(s.Default, min) >= 0 {
		return ""
	}
	return min
}

func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, _ := strconv.Atoi(as[i])
		y, _ := strconv.Atoi(bs[i])
		if x != y {
			return x - y
		}
	}
	return 0
}

// packageJSONStacks: Node.js (volta's pin, else engines), and Bun when the package manager or
// engines name it.
func packageJSONStacks(data []byte) map[string]string {
	var m struct {
		Engines        map[string]string `json:"engines"`
		Volta          map[string]string `json:"volta"`
		PackageManager string            `json:"packageManager"`
	}
	if json.Unmarshal(data, &m) != nil {
		return map[string]string{"node": ""}
	}
	out := map[string]string{"node": ""}
	switch r := m.Engines["node"]; {
	case m.Volta["node"] != "":
		out["node"] = nodeMajorOrExact(m.Volta["node"])
	case openRange(r):
		out["node"] = atLeast("node", nodeMajorOrExact(r))
	case r != "":
		out["node"] = nodeRange(r)
	}
	if v, ok := strings.CutPrefix(m.PackageManager, "bun@"); ok {
		v, _, _ = strings.Cut(v, "+")
		out["bun"] = v
	} else if r, ok := m.Engines["bun"]; ok {
		out["bun"] = versionRE.FindString(r)
	}
	return out
}

var (
	requiresPythonRE = regexp.MustCompile(`^requires-python\s*=\s*["']([^"']+)["']`)
	poetryPythonRE   = regexp.MustCompile(`^python\s*=\s*["']([^"']+)["']`)
	minorRE          = regexp.MustCompile(`3\.\d+`)
)

// requiresPython reads pyproject.toml's requires-python (or Poetry's python): an open range
// (">=3.11", "^3.11") takes the default when it fits, else the named 3.x.
func requiresPython(data []byte) string {
	for _, line := range lines(data) {
		m := requiresPythonRE.FindStringSubmatch(line)
		if m == nil {
			m = poetryPythonRE.FindStringSubmatch(line)
		}
		if m == nil {
			continue
		}
		v := minorRE.FindString(m[1])
		if v == "" {
			return ""
		}
		if r := strings.TrimSpace(m[1]); openRange(r) || strings.HasPrefix(r, "^") {
			return atLeast("python", v)
		}
		return v
	}
	return ""
}

// goVersion reads go.mod or go.work: the toolchain line, else the go line.
func goVersion(data []byte) string {
	var goLine string
	for _, line := range lines(data) {
		if v, ok := strings.CutPrefix(line, "toolchain go"); ok {
			return strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "go "); ok && goLine == "" {
			goLine = strings.TrimSpace(v)
		}
	}
	return goLine
}

var channelRE = regexp.MustCompile(`^channel\s*=\s*["']([^"']+)["']`)

// rustChannel reads rust-toolchain.toml's channel, or the plain rust-toolchain file.
func rustChannel(data []byte) string {
	for _, line := range lines(data) {
		if m := channelRE.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	if v := firstLine(data); !strings.HasPrefix(v, "[") {
		return v
	}
	return ""
}

// toolVersions reads asdf's and mise's .tool-versions ("nodejs 22.11.0").
func toolVersions(data []byte) map[string]string {
	names := map[string]string{"nodejs": "node", "node": "node", "python": "python", "golang": "go", "go": "go",
		"rust": "rust", "bun": "bun", "java": "java", "ruby": "ruby"}
	out := map[string]string{}
	for _, line := range lines(data) {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if stack, ok := names[f[0]]; ok {
			v := f[1]
			switch stack {
			case "node":
				v = nodeMajorOrExact(v)
			case "java":
				v = javaMajor(v)
			case "ruby":
				v = "" // the system's
			}
			out[stack] = v
		}
	}
	return out
}
