package webserver

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/common/version"
)

/* This file has the methods that pass out actual content. */

// Returns the main index file.
// If index.html becomes a template, this is where it can be compiled.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	index := filepath.Join(s.HTMLPath, "index.html")
	http.ServeFile(w, r, index)
}

// Arbitrary /health handler.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.handleDone(w, []byte("OK"), mimeHTML)
}

// Returns static files from static-files path. /css, /js, /img (/images, /image).
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	switch v := mux.Vars(r)["sub"]; v {
	case "image", "img":
		dir := http.Dir(filepath.Join(s.HTMLPath, "static", "images"))
		http.StripPrefix("/"+v, http.FileServer(dir)).ServeHTTP(w, r)
	default: // images, js, css, etc
		dir := http.Dir(filepath.Join(s.HTMLPath, "static", v))
		http.StripPrefix("/"+v, http.FileServer(dir)).ServeHTTP(w, r)
	}
}

// Returns poller configs and/or plugins. Returns compiled go package names and versions. /api/v1/config.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	switch vars["sub"] {
	case "":
		var m runtime.MemStats

		runtime.ReadMemStats(&m)

		p := s.getPlugins()
		data := map[string]interface{}{
			"inputs":  p["inputs"],
			"outputs": p["outputs"],
			"poller":  s.Collect.Poller(),
			"gover":   runtime.Version(),
			"version": version.Version,
			"branch":  version.Branch,
			"built":   version.BuildDate,
			"cpus":    runtime.NumCPU(),
			"arch":    runtime.GOOS + " " + runtime.GOARCH,
			"uptime":  int(time.Since(s.start).Round(time.Second).Seconds()),
			"uid":     os.Getuid(),
			"pid":     os.Getpid(),
			"gid":     os.Getgid(),
			"malloc":  m.Alloc,
			"mtalloc": m.TotalAlloc,
			"memsys":  m.Sys,
			"numgc":   m.NumGC,
		}
		s.handleJSON(w, data)
	case "plugins":
		s.handleJSON(w, s.getPlugins())
	default:
		s.handleMissing(w, r)
	}
}

// getPlugins merges the plugin names, paths and versions from input data and runtime data.
func (s *Server) getPlugins() map[string]map[string]interface{} {
	type plugin struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Path    string `json:"path"`
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok || bi == nil {
		bi = &debug.BuildInfo{Deps: []*debug.Module{}}
	}

	i := map[string]map[string]interface{}{
		"inputs":  make(map[string]interface{}),
		"outputs": make(map[string]interface{}),
	}

	parse := func(name string, values map[string]string) {
		for k, v := range values {
			for l := range bi.Deps {
				if bi.Deps[l].Path == v {
					i[name][k] = plugin{Name: k, Path: v, Version: bi.Deps[l].Version}
					break
				}
			}
		}
	}

	parse("inputs", s.Collect.Inputs(""))
	parse("outputs", s.Collect.Outputs(""))

	return i
}

// Returns an output plugin's data: /api/v1/output/{output}.
func (s *Server) handleOutput(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	c := s.plugins.getOutput(vars["output"])
	if c == nil {
		s.handleMissing(w, r)
		return
	}

	c.RLock()
	defer c.RUnlock()

	switch val := vars["value"]; vars["sub"] {
	default:
		s.handleJSON(w, c.Config)
	case "eventgroups":
		s.handleJSON(w, c.Events.Groups(val))
	case "events":
		switch events, ok := c.Events[val]; {
		case val == "":
			s.handleJSON(w, c.Events)
		case ok:
			s.handleJSON(w, events)
		default:
			s.handleMissing(w, r)
		}
	case "counters":
		if val == "" {
			s.handleJSON(w, c.Counter)
		} else {
			s.handleJSON(w, map[string]int64{val: c.Counter[val]})
		}
	}
}

// Returns an input plugin's data: /api/v1/input/{input}.
func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	c := s.plugins.getInput(vars["input"])
	if c == nil {
		s.handleMissing(w, r)
		return
	}

	c.RLock()
	defer c.RUnlock()

	switch val := vars["value"]; vars["sub"] {
	default:
		s.handleJSON(w, c.Config)
	case "eventgroups":
		s.handleJSON(w, c.Events.Groups(val))
	case "events":
		switch events, ok := c.Events[val]; {
		case val == "":
			s.handleJSON(w, c.Events)
		case ok:
			s.handleJSON(w, events)
		default:
			s.handleMissing(w, r)
		}
	case "sites":
		s.handleJSON(w, c.Sites)
	case "devices":
		s.handleJSON(w, c.Devices.Filter(val))
	case "clients":
		s.handleJSON(w, c.Clients.Filter(val))
	case "counters":
		if val != "" {
			s.handleJSON(w, map[string]int64{val: c.Counter[val]})
		} else {
			s.handleJSON(w, c.Counter)
		}
	}
}
