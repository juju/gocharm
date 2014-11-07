package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/utils/parallel"
	"gopkg.in/juju/charm.v4"
)

var (
	destDir  string
	cacheDir = flag.String("cache", filepath.Join(os.Getenv("HOME"), ".juju", "charmcache"), "charm cache directory")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: getallcharms charm-directory\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
	}
	destDir = flag.Arg(0)
	all, err := allURLs()
	if err != nil {
		log.Fatalf("cannot fetch all charm URLs: %v", err)
	}
	charm.CacheDir = *cacheDir
	if err := os.Mkdir(destDir, 0777); err != nil {
		log.Fatalf("cannot make destination directory: %v", err)
	}
	listFile, err := os.Create(filepath.Join(destDir, "urls.txt"))
	if err != nil {
		log.Fatal(err)
	}
	defer listFile.Close()
	urlDone := make(chan *charm.URL)
	go download(urlDone, all)
	for url := range urlDone {
		fmt.Fprintf(listFile, "%v\n", strings.TrimPrefix(url.String(), "cs:"))
	}
}

func download(urlDone chan<- *charm.URL, all []string) {
	par := parallel.NewRun(20)
	for _, name := range all {
		name := name
		par.Do(func() error {
			curl, err := charm.ParseURL(name)
			if err != nil {
				log.Printf("cannot infer URL from %q: %v", name, err)
				return err
			}
			dir := filepath.Join(destDir, strings.TrimPrefix(curl.String(), "cs:"))
			if _, err := os.Stat(dir); err == nil {
				urlDone <- curl
				log.Printf("%q is already downloaded", dir)
				return nil
			}
			fmt.Printf("%s\n", curl)
			ch, err := charm.Store.Get(curl)
			if err != nil {
				log.Printf("get %q failed: %v", curl, err)
				return err
			}
			bun := ch.(*charm.CharmArchive)
			err = bun.ExpandTo(dir)
			if err != nil {
				log.Printf("expand %q to %q failed: %v", curl, dir, err)
				return err
			}
			urlDone <- curl
			return nil
		})
	}
	par.Wait()
	close(urlDone)
}

type results struct {
	Results []result `json:"result"`
}

type result struct {
	Charm charmInfo `json:"charm"`
}

type charmInfo struct {
	URL string `json:"url"`
}

func allURLs() ([]string, error) {
	resp, err := http.Get("http://manage.jujucharms.com/api/2/charms?text=")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad http response %q", resp.Status)
	}
	dec := json.NewDecoder(resp.Body)
	var res results
	if err := dec.Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode results: %v", err)
	}
	var all []string
	for _, r := range res.Results {
		all = append(all, r.Charm.URL)
	}
	return all, nil
}
