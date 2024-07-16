package tools

import (
	"net/http"
	_ "net/http/pprof"
)

// Service analysis tools
// go tool pprof -http=:7778 -seconds=20 http://localhost:7777/debug/pprof/profile
func PProf() {
	go func() {
		err := http.ListenAndServe(":7777", nil)
		if err != nil {
			panic(err)
		}
	}()
}
