package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// State distinguishes an explicit argument from an unobserved default. Values
// are not inferred from this controller's runtime.GOMAXPROCS.
type executionParameter struct {
	State  string `json:"state"`
	Value  string `json:"value,omitempty"`
	Source string `json:"source"`
}

type executionParameters struct {
	Kind            string             `json:"command_kind"`
	Status          string             `json:"status"`
	Count           executionParameter `json:"count"`
	Timeout         executionParameter `json:"timeout"`
	TestParallel    executionParameter `json:"test_parallelism"`
	PackageParallel executionParameter `json:"go_package_parallelism"`
	Tags            executionParameter `json:"build_tags"`
	Filter          executionParameter `json:"test_filter"`
	Skip            executionParameter `json:"test_skip"`
	GOMAXPROCS      executionParameter `json:"gomaxprocs_environment"`
	GOFLAGS         string             `json:"goflags_environment"`
	BuildCommand    []string           `json:"build_command,omitempty"`
}

func parameter(state, value, source string) executionParameter {
	return executionParameter{State: state, Value: value, Source: source}
}

func executionParametersFor(command, build []string) executionParameters {
	u := parameter("unknown", "", "command unavailable")
	p := executionParameters{Kind: "unknown", Status: "not_executed", Count: u, Timeout: u, TestParallel: u, PackageParallel: u, Tags: u, Filter: u, Skip: u, GOMAXPROCS: parameter("unset", "", "process environment"), GOFLAGS: os.Getenv("GOFLAGS")}
	if value, ok := os.LookupEnv("GOMAXPROCS"); ok {
		p.GOMAXPROCS = parameter("explicit", value, "process environment")
	}
	if len(command) == 0 {
		return p
	}
	app := strings.TrimSuffix(strings.ToLower(filepath.Base(command[0])), ".exe")
	var args []string
	if app == "go" && len(command) > 1 && command[1] == "test" {
		p.Kind, args = "go_test", command[2:]
	} else if app == "go" && len(command) > 2 && command[1] == "tool" && command[2] == "test2json" {
		p.Kind = "test2json"
		// -p here names the JSON package; it is not Go build parallelism.
		index := 3
		for index < len(command) && strings.HasPrefix(command[index], "-") {
			if command[index] == "-p" {
				index++
			}
			index++
		}
		if index >= len(command) {
			return p // converter only; no test process was specified
		}
		args = command[index+1:]
	} else if app == "node" || app == "npm" || app == "npm.cmd" {
		p.Kind = "web"
		na := parameter("not_applicable", "", "non-Go command")
		p.Count, p.Timeout, p.TestParallel, p.PackageParallel, p.Tags, p.Filter = na, na, na, na, na, na
		p.Skip = na
		return p
	} else if strings.Contains(app, ".test") || len(build) > 0 {
		p.Kind, args = "test_binary", command[1:]
	} else {
		return p
	}
	flags, complete, opaque := executionFlags(args, p.Kind == "go_test")
	lookup := func(name, source string) executionParameter {
		if value, ok := flags[name]; ok {
			return parameter("explicit", value, source)
		}
		if !complete {
			return parameter("unknown", "", "incomplete command flags")
		}
		if p.Kind == "go_test" && p.GOFLAGS != "" {
			return parameter("unknown", "", "GOFLAGS environment; not expanded")
		}
		return parameter("default", "", source+" default; not measured")
	}
	p.Count, p.Timeout = lookup("count", "test argv"), lookup("timeout", "test argv")
	p.TestParallel, p.Filter = lookup("parallel", "test argv"), lookup("run", "test argv")
	p.Skip = lookup("skip", "test argv")
	if opaque != "" {
		unknown := parameter("unknown", "", "opaque custom arguments; test flag effects not expanded")
		p.Count, p.Timeout, p.TestParallel, p.Filter, p.Skip = unknown, unknown, unknown, unknown, unknown
	}
	if p.Kind == "go_test" {
		p.PackageParallel, p.Tags = lookup("p", "go argv"), lookup("tags", "go argv")
		if opaque == "unknown_flag" {
			p.PackageParallel, p.Tags = parameter("unknown", "", "unknown flag arity"), parameter("unknown", "", "unknown flag arity")
		}
		compileOnly, _ := strconv.ParseBool(flags["c"])
		if compileOnly {
			p.Kind = "go_test_compile"
			na := parameter("not_applicable", "", "compile-only command")
			p.Count, p.Timeout, p.TestParallel, p.Filter = na, na, na, na
			p.Skip = na
		}
	} else {
		p.PackageParallel = parameter("not_applicable", "", "already compiled test binary")
		p.Tags = parameter("unknown", "", "build command unavailable")
		if len(build) > 0 {
			p.BuildCommand = append([]string(nil), build...)
			compiled := executionParametersFor(build, nil)
			p.Tags = compiled.Tags
			p.Tags.Source = "build command: " + p.Tags.Source
		}
	}
	return p
}

// Consume values once, so -run "-count=2" is a filter rather than a second
// count flag. Go's -args and -- terminate Go/test-flag interpretation.
func executionFlags(args []string, goTest bool) (map[string]string, bool, string) {
	values := map[string]bool{}
	for _, name := range strings.Fields("p parallel count timeout run skip tags o bench benchtime cpu list fuzz fuzztime fuzzminimizetime coverprofile coverpkg covermode blockprofile blockprofilerate cpuprofile memprofile memprofilerate mutexprofile mutexprofilefraction outputdir trace vet exec asmflags buildmode compiler gccgoflags gcflags installsuffix ldflags mod modfile overlay pkgdir toolexec coverdir godebug shuffle testlogfile gocoverdir") {
		values[name] = true
	}
	booleans := map[string]bool{}
	for _, name := range strings.Fields("c i json v short failfast fullpath race msan asan cover benchmem work n x a linkshared modcacherw trimpath buildvcs paniconexit0") {
		booleans[name] = true
	}
	testFlags := map[string]bool{}
	for _, name := range strings.Fields("v short failfast fullpath bench benchtime benchmem count timeout parallel run skip cpu list fuzz fuzztime fuzzminimizetime coverprofile blockprofile blockprofilerate cpuprofile memprofile memprofilerate mutexprofile mutexprofilefraction outputdir trace shuffle paniconexit0 testlogfile gocoverdir") {
		testFlags[name] = true
	}
	flags := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-args" || arg == "--" {
			return flags, true, "forwarded"
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		key, value, equals := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if strings.HasPrefix(key, "test.") {
			key = strings.TrimPrefix(key, "test.")
			if !testFlags[key] {
				return flags, false, "unknown_flag"
			}
		} else if !goTest {
			return flags, false, "unknown_flag"
		}
		if !values[key] && !booleans[key] {
			return flags, false, "unknown_flag"
		}
		if values[key] && !equals {
			if i+1 >= len(args) {
				return flags, false, ""
			}
			i++
			value = args[i]
		} else if !equals {
			value = "true"
		}
		flags[key] = value
	}
	return flags, true, ""
}

func finishExecutionParameters(p *executionParameters, executed bool) {
	if executed {
		p.Status = "executed"
	} else {
		p.Status = "attempted"
	}
}

func summarizeExecutionParameters(env *environment, groups []scopedGroup) {
	if env.Concurrency == nil {
		env.Concurrency = map[string]string{}
	}
	for key, field := range map[string]func(executionParameters) executionParameter{
		"go_test_parallel":          func(p executionParameters) executionParameter { return p.TestParallel },
		"go_build_package_parallel": func(p executionParameters) executionParameter { return p.PackageParallel },
	} {
		value, seen := "unknown", false
		for _, group := range groups {
			p := group.ExecutionParameters
			v := field(p)
			if v.State == "not_applicable" {
				continue
			}
			rendered := v.State
			if p.Status != "executed" || v.State == "" {
				rendered = "unknown"
			} else if v.State == "explicit" {
				rendered = v.Value
			}
			if seen && rendered != value {
				value = "mixed"
			} else if !seen {
				value = rendered
			}
			seen = true
		}
		if len(groups) > 0 && !seen {
			value = "not_applicable"
		}
		env.Concurrency[key] = value
	}
}

func attachBuildParameters(g *scopedGroup, path string) {
	raw, err := os.ReadFile(path)
	var command []string
	if err == nil {
		err = json.Unmarshal(raw, &command)
	}
	if err != nil || len(command) == 0 {
		g.Error += "; build command evidence unavailable: " + path
		return
	}
	status := g.ExecutionParameters.Status
	g.ExecutionParameters = executionParametersFor(g.Command, command)
	g.ExecutionParameters.Status = status
}
