package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/gosimple/slug"
	"github.com/mmcdole/gofeed"
)

var queryString string
var downloadFlag bool
var outputPath string
var serverModeFlag bool
var queryListPath string
var minimumVideoLenMinutes uint64
var parallelDownloadsNum uint64
var interval uint64

const baseUrl string = "https://mediathekviewweb.de/feed?query=%s&everywhere=true&future=false"

var seen []Job

func init() {
	flag.StringVar(&queryString, "query", "", "search query for the mediathek")
	flag.StringVar(&outputPath, "output", "./output", "output path (will be created if not exists)")
	flag.BoolVar(&downloadFlag, "download", false, "download all matches")
	flag.BoolVar(&serverModeFlag, "server", false, "start server mode")
	flag.StringVar(&queryListPath, "query-file", "", "path to a file containing query strings")
	flag.Uint64Var(&minimumVideoLenMinutes, "min", 20, "the minimum video length in minutes")
	flag.Uint64Var(&parallelDownloadsNum, "parallel", uint64(runtime.NumCPU()), "number of parallel downloads")
	flag.Uint64Var(&interval, "interval", 60, "fetch interval in seconds (for server mode)")

	flag.Parse()
}

type Job struct {
	url       string
	fileName  string
	outputDir string
}

func downloader(jobs <-chan *Job, wg *sync.WaitGroup) {
	client := grab.NewClient()
	for job := range jobs {
		if err := os.MkdirAll(job.outputDir, os.ModePerm); err != nil {
			log.Fatal(err)
		}

		dst := filepath.Join(job.outputDir, job.fileName+".mp4")
		req, err := grab.NewRequest(dst, job.url)
		if err != nil {
			log.Fatal(err)
		}
		resp := client.Do(req)
		<-resp.Done
	}
	wg.Done()
}

func expandDir(dir string) (string, error) {
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		} else {
			return filepath.Join(home, dir[2:]), nil
		}
	}
	return dir, nil
}

func alreadyDownloaded(job Job) bool {
	for _, knownJob := range seen {
		if job == knownJob {
			return true
		}
	}
	return false
}

func fetch(urls []string) {
	var jobs []Job

	for _, url := range urls {
		fp := gofeed.NewParser()
		feed, _ := fp.ParseURL(url)

		for _, item := range feed.Items {
			if videoLen, err := strconv.Atoi(item.Custom["duration"]); err == nil {
				if len(item.Enclosures) > 0 {
					video := item.Enclosures[0]

					if uint64(videoLen/60) >= minimumVideoLenMinutes {
						if serverModeFlag || downloadFlag {
							out, expandErr := expandDir(outputPath)
							if expandErr != nil {
								log.Fatal(expandErr)
							}
							j := Job{
								url:       video.URL,
								fileName:  slug.Make(item.Title),
								outputDir: out,
							}

							if !alreadyDownloaded(j) {
								fmt.Printf("Downloading \"%s\"\n", item.Title)
								seen = append(seen, j)
								jobs = append(jobs, j)
							}
						} else {
							fmt.Printf("%s | %d mins\n", item.Title, uint64(videoLen/60))
						}
					}
				}
			}
		}
	}

	if serverModeFlag || downloadFlag {
		if len(jobs) > 0 {
			reqs := make([]*grab.Request, 0)

			for _, job := range jobs {
				if err := os.MkdirAll(job.outputDir, os.ModePerm); err != nil {
					log.Fatal(err)
				}
				dst := filepath.Join(job.outputDir, job.fileName+".mp4")
				req, err := grab.NewRequest(dst, job.url)
				if err == nil {

					reqs = append(reqs, req)
				}

			}

			client := grab.NewClient()
			resp := client.DoBatch(int(parallelDownloadsNum), reqs...)
			for r := range resp {
				if err := r.Err(); err != nil {
					log.Fatal(err)
				}
			}
		}
	}
}

func formatQuery(q string) string {
	return fmt.Sprintf(baseUrl, url.QueryEscape(q))
}

func main() {
	var queries []string

	if queryString != "" {
		queryUrl := formatQuery(queryString)
		queries = append(queries, queryUrl)
	}

	if queryListPath != "" {
		queryFile, err := os.Open(queryListPath)
		if err != nil {
			log.Fatal(err)
		}
		defer queryFile.Close()

		scanner := bufio.NewScanner(queryFile)
		var q string
		for scanner.Scan() {
			q = formatQuery(strings.TrimSpace(scanner.Text()))
			queries = append(queries, q)
		}
	}

	if serverModeFlag {
		for {
			fetch(queries)
			time.Sleep(time.Duration(interval) * time.Second)
		}
	} else {
		fetch(queries)
	}
}
